package watcher

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/iweka-dev/webhook-hub/internal/config"
	"github.com/iweka-dev/webhook-hub/internal/domain"
	"github.com/iweka-dev/webhook-hub/internal/ingress"
)

const (
	headerWebhookID        = "webhook-id"
	headerWebhookTimestamp = "webhook-timestamp"
	headerWebhookSignature = "webhook-signature"
	headerEventID          = "X-Watcher-Event-ID"
	headerDeliveryID       = "X-Watcher-Delivery-ID"
	headerEventType        = "X-Watcher-Event"
	headerTimestamp        = "X-Watcher-Timestamp"
	headerSignature        = "X-Watcher-Signature"
)

type Adapter struct{}

type Payload struct {
	SchemaVersion any    `json:"schema_version"`
	EventID       string `json:"event_id"`
	EventType     string `json:"event_type"`
	OccurredAt    string `json:"occurred_at"`
	Summary       string `json:"summary"`
	TriggeredBy   string `json:"triggered_by"`
	Watcher       struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"watcher"`
	Version struct {
		DiscoveredVersion string `json:"discovered_version"`
		CurrentVersion    string `json:"current_version"`
		WillDeploy        bool   `json:"will_deploy"`
		BlockReason       string `json:"block_reason"`
	} `json:"version"`
	Attempt struct {
		ID                  int64  `json:"id"`
		Kind                string `json:"kind"`
		Reason              string `json:"reason"`
		TriggeredBy         string `json:"triggered_by"`
		Status              string `json:"status"`
		TargetVersion       string `json:"target_version"`
		FromVersion         string `json:"from_version"`
		FailedTargetVersion string `json:"failed_target_version"`
		FailurePhase        string `json:"failure_phase"`
		Error               string `json:"error"`
		ParentAttemptID     *int64 `json:"parent_attempt_id"`
		RootAttemptID       int64  `json:"root_attempt_id"`
	} `json:"attempt"`
	Service struct {
		ID             int64  `json:"id"`
		Name           string `json:"name"`
		ServiceType    string `json:"service_type"`
		HealthCheckURL string `json:"health_check_url"`
	} `json:"service"`
	Health struct {
		PreviousStatus string `json:"previous_status"`
		CurrentStatus  string `json:"current_status"`
		HTTPStatus     int    `json:"http_status"`
		Error          string `json:"error"`
		CheckedAt      string `json:"checked_at"`
		Source         string `json:"source"`
	} `json:"health"`
	FailedDelivery struct {
		EventID            string `json:"event_id"`
		EventType          string `json:"event_type"`
		DeliveryID         string `json:"delivery_id"`
		AttemptNumber      int    `json:"attempt_number"`
		ResponseStatusCode int    `json:"response_status_code"`
		Error              string `json:"error"`
		Summary            string `json:"summary"`
	} `json:"failed_delivery"`

	LegacyID          string
	LegacyEvent       string
	LegacyService     string
	LegacyEnvironment string
	LegacyVersion     string
	LegacyCommitSHA   string
	LegacyActor       string
	LegacyURL         string
	LegacyLabels      map[string]string
	LegacyError       struct {
		Message string `json:"message"`
		Stage   string `json:"stage"`
	}
}

type legacyPayload struct {
	SchemaVersion any               `json:"schema_version"`
	ID            string            `json:"id"`
	Event         string            `json:"event"`
	OccurredAt    time.Time         `json:"occurred_at"`
	Service       string            `json:"service"`
	Environment   string            `json:"environment"`
	Version       string            `json:"version"`
	CommitSHA     string            `json:"commit_sha"`
	Actor         string            `json:"actor"`
	URL           string            `json:"url"`
	Labels        map[string]string `json:"labels"`
	Error         struct {
		Message string `json:"message"`
		Stage   string `json:"stage"`
	} `json:"error"`
}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Source() domain.Source {
	return domain.SourceWatcher
}

func (a *Adapter) Verify(_ context.Context, integration config.IntegrationConfig, req ingress.InboundRequest) error {
	timestamp, signature, webhookID := standardWebhookHeaders(req.Headers)
	if signature == "" {
		timestamp = req.Headers.Get(headerTimestamp)
		signature = req.Headers.Get(headerSignature)
		webhookID = req.Headers.Get(headerEventID)
	}
	if timestamp == "" || signature == "" || integration.ResolvedSecret == "" {
		return ingress.ErrUnauthorized
	}

	parsed, err := parseWebhookTimestamp(timestamp)
	if err != nil {
		return ingress.ErrUnauthorized
	}
	if integration.ReplayWindow > 0 && absDuration(time.Since(parsed.UTC())) > integration.ReplayWindow {
		return ingress.ErrUnauthorized
	}

	key := watcherSigningKey(integration.ResolvedSecret)
	if len(key) == 0 {
		return ingress.ErrUnauthorized
	}
	mac := hmac.New(sha256.New, key)
	if webhookID != "" {
		mac.Write([]byte(webhookID))
		mac.Write([]byte("."))
		mac.Write([]byte(timestamp))
		mac.Write([]byte("."))
	} else {
		mac.Write([]byte(timestamp))
		mac.Write([]byte(":"))
	}
	mac.Write(req.RawBody)
	expected := mac.Sum(nil)
	if !matchesWebhookSignature(signature, expected) {
		return ingress.ErrUnauthorized
	}

	return nil
}

func (a *Adapter) Normalize(_ context.Context, integrationID string, _ config.IntegrationConfig, req ingress.InboundRequest) (ingress.AdapterResult, error) {
	var p Payload

	var legacy legacyPayload
	payloadErr := json.Unmarshal(req.RawBody, &p)
	legacyErr := json.Unmarshal(req.RawBody, &legacy)

	if payloadErr != nil && legacyErr != nil {
		return ingress.AdapterResult{}, ingress.ErrMalformedPayload
	}

	if p.EventID == "" && p.EventType == "" && (legacy.ID != "" || legacy.Event != "") {
		p = fromLegacyPayload(legacy)
	}

	if p.EventID == "" && p.LegacyID == "" {
		return ingress.AdapterResult{}, ingress.ErrMalformedPayload
	}

	eventType := canonicalWatcherEventType(firstNonEmpty(p.EventType, p.LegacyEvent, req.Headers.Get(headerEventType)))
	if eventType == "" {
		return ingress.AdapterResult{}, ingress.ErrMalformedPayload
	}

	occurredAt := parseTimestamp(firstNonEmpty(p.OccurredAt, legacy.OccurredAt.Format(time.RFC3339)), req.ReceivedAt)
	serviceName := firstNonEmpty(p.Service.Name, p.LegacyService)
	release := firstNonEmpty(p.Attempt.TargetVersion, p.Version.DiscoveredVersion, p.LegacyVersion)
	actor := firstNonEmpty(p.Attempt.TriggeredBy, p.TriggeredBy, p.LegacyActor)
	url := firstNonEmpty(p.LegacyURL, p.Service.HealthCheckURL)
	sourceEventID := firstNonEmpty(p.EventID, p.LegacyID, req.Headers.Get(headerEventID))
	sourceDeliveryID := firstNonEmpty(req.Headers.Get(headerWebhookID), req.Headers.Get(headerDeliveryID), sourceEventID, sourceEventID+":"+eventType)

	fields := map[string]any{
		"schema_version":  p.SchemaVersion,
		"event_type":      eventType,
		"watcher_id":      p.Watcher.ID,
		"watcher_name":    p.Watcher.Name,
		"summary":         p.Summary,
		"triggered_by":    p.TriggeredBy,
		"version":         p.Version,
		"attempt":         p.Attempt,
		"service":         p.Service,
		"health":          p.Health,
		"failed_delivery": p.FailedDelivery,
	}
	if p.LegacyError.Message != "" || p.LegacyError.Stage != "" {
		fields["legacy_error_message"] = p.LegacyError.Message
		fields["legacy_error_stage"] = p.LegacyError.Stage
	}

	labelsJSON, _ := json.Marshal(p.LegacyLabels)
	fieldsJSON, _ := json.Marshal(fields)

	event := domain.Event{
		Source:        domain.SourceWatcher,
		IntegrationID: integrationID,
		SourceEventID: sourceEventID,
		Type:          eventType,
		Action:        watcherAction(eventType),
		Lifecycle:     watcherLifecycle(eventType),
		Severity:      watcherSeverity(eventType, p),
		Title:         firstNonEmpty(p.Summary, fallbackWatcherTitle(eventType, p)),
		Summary:       firstNonEmpty(p.Summary, fallbackWatcherSummary(eventType, serviceName, p)),
		Service:       serviceName,
		Environment:   p.LegacyEnvironment,
		Release:       release,
		CommitSHA:     p.LegacyCommitSHA,
		Actor:         actor,
		Fingerprint:   watcherFingerprint(sourceEventID, eventType, p),
		URL:           firstNonEmpty(url, p.Service.HealthCheckURL),
		OccurredAt:    occurredAt,
		LabelsJSON:    labelsJSON,
		FieldsJSON:    fieldsJSON,
	}

	return ingress.AdapterResult{
		SourceDeliveryID: sourceDeliveryID,
		SourceEventType:  eventType,
		Events:           []domain.Event{event},
	}, nil
}

func fromLegacyPayload(in legacyPayload) Payload {
	out := Payload{
		SchemaVersion:     in.SchemaVersion,
		OccurredAt:        in.OccurredAt.UTC().Format(time.RFC3339),
		LegacyID:          in.ID,
		LegacyEvent:       in.Event,
		LegacyService:     in.Service,
		LegacyEnvironment: in.Environment,
		LegacyVersion:     in.Version,
		LegacyCommitSHA:   in.CommitSHA,
		LegacyActor:       in.Actor,
		LegacyURL:         in.URL,
		LegacyLabels:      in.Labels,
	}
	out.LegacyError.Message = in.Error.Message
	out.LegacyError.Stage = in.Error.Stage
	return out
}

func standardWebhookHeaders(headers http.Header) (timestamp, signature, webhookID string) {
	return strings.TrimSpace(headers.Get(headerWebhookTimestamp)), strings.TrimSpace(headers.Get(headerWebhookSignature)), strings.TrimSpace(headers.Get(headerWebhookID))
}

func parseWebhookTimestamp(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), nil
	}
	parsedUnix, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(parsedUnix, 0).UTC(), nil
}

func matchesWebhookSignature(signature string, expected []byte) bool {
	encodedCandidates := strings.Fields(signature)
	if len(encodedCandidates) == 0 {
		encodedCandidates = []string{signature}
	}
	expectedBase64 := base64.StdEncoding.EncodeToString(expected)
	for _, candidate := range encodedCandidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.Contains(candidate, ",") {
			parts := strings.SplitN(candidate, ",", 2)
			candidate = parts[len(parts)-1]
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(expectedBase64)) == 1 {
			return true
		}
	}
	return false
}

func watcherSigningKey(secret string) []byte {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	if strings.HasPrefix(secret, "whsec_") {
		if decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_")); err == nil {
			return decoded
		}
	}
	return []byte(secret)
}

func canonicalWatcherEventType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Count(value, ".") == 1 {
		parts := strings.SplitN(value, ".", 2)
		value = parts[0] + "." + strings.ReplaceAll(parts[1], "_", ".")
	} else {
		value = strings.ReplaceAll(value, "_", ".")
	}
	value = strings.ReplaceAll(value, ".rolled.back", ".rolled_back")
	if strings.HasPrefix(value, "watcher.") || strings.HasPrefix(value, "service.") || strings.HasPrefix(value, "webhook.") {
		return value
	}
	return "watcher." + strings.ReplaceAll(value, "_", ".")
}

func watcherSeverity(eventType string, p Payload) domain.Severity {
	switch {
	case strings.Contains(eventType, "rollback.failed"), strings.Contains(eventType, "delivery.exhausted"):
		return domain.SeverityCritical
	case strings.Contains(eventType, "failed"), strings.Contains(eventType, "unhealthy"):
		return domain.SeverityError
	case strings.Contains(eventType, "cancelled"):
		return domain.SeverityWarning
	case strings.Contains(eventType, "webhook.test"):
		return domain.SeverityInfo
	case strings.Contains(eventType, "health.changed"):
		if strings.EqualFold(p.Health.CurrentStatus, "unhealthy") || p.Health.HTTPStatus >= 500 {
			return domain.SeverityError
		}
		return domain.SeverityWarning
	default:
		return domain.SeverityInfo
	}
}

func watcherLifecycle(eventType string) domain.Lifecycle {
	switch {
	case strings.Contains(eventType, "started"):
		return domain.LifecycleStarted
	case strings.Contains(eventType, "found"), strings.Contains(eventType, "test"):
		return domain.LifecycleTriggered
	case strings.Contains(eventType, "changed"):
		return domain.LifecycleUpdated
	case strings.Contains(eventType, "succeeded"):
		return domain.LifecycleSucceeded
	case strings.Contains(eventType, "failed"):
		return domain.LifecycleFailed
	case strings.Contains(eventType, "resolved"), strings.Contains(eventType, "healthy"):
		return domain.LifecycleResolved
	case strings.Contains(eventType, "cancelled"):
		return domain.LifecycleCancelled
	case strings.Contains(eventType, "rolled_back"):
		return domain.LifecycleRolledBack
	default:
		return domain.LifecycleTriggered
	}
}

func watcherAction(eventType string) string {
	parts := strings.Split(eventType, ".")
	if len(parts) == 0 {
		return eventType
	}
	return parts[len(parts)-1]
}

func fallbackWatcherTitle(eventType string, p Payload) string {
	switch {
	case p.Version.DiscoveredVersion != "":
		return fmt.Sprintf("Version found: %s", p.Version.DiscoveredVersion)
	case p.Attempt.TargetVersion != "":
		return strings.ReplaceAll(eventType, ".", " ")
	case p.Health.CurrentStatus != "":
		return fmt.Sprintf("Health changed: %s", p.Health.CurrentStatus)
	case p.FailedDelivery.EventID != "":
		return fmt.Sprintf("Webhook delivery exhausted: %s", p.FailedDelivery.EventType)
	default:
		return strings.ReplaceAll(eventType, ".", " ")
	}
}

func watcherFingerprint(sourceEventID, eventType string, p Payload) string {
	if sourceEventID != "" {
		return sourceEventID
	}
	if p.Attempt.RootAttemptID > 0 {
		return fmt.Sprintf("%s:%d", eventType, p.Attempt.RootAttemptID)
	}
	if p.FailedDelivery.EventID != "" {
		return p.FailedDelivery.EventID
	}
	return eventType
}

func fallbackWatcherSummary(eventType, serviceName string, p Payload) string {
	switch {
	case p.Version.DiscoveredVersion != "":
		if p.Version.BlockReason != "" {
			return fmt.Sprintf("Watcher found %s but rollout is blocked: %s", p.Version.DiscoveredVersion, p.Version.BlockReason)
		}
		return fmt.Sprintf("Watcher found version %s", p.Version.DiscoveredVersion)
	case p.Attempt.Status != "":
		return firstNonEmpty(p.Summary, p.Attempt.Error, p.Attempt.Status)
	case p.Health.CurrentStatus != "":
		return firstNonEmpty(p.Summary, fmt.Sprintf("Service health is now %s", p.Health.CurrentStatus))
	case p.FailedDelivery.EventID != "":
		return firstNonEmpty(p.Summary, p.FailedDelivery.Summary)
	case p.LegacyError.Message != "":
		return p.LegacyError.Message
	case p.Attempt.Error != "":
		return p.Attempt.Error
	case serviceName != "":
		return fmt.Sprintf("%s %s", serviceName, watcherAction(eventType))
	default:
		return strings.ReplaceAll(eventType, ".", " ")
	}
}

func parseTimestamp(value string, fallback time.Time) time.Time {
	if parsed, err := parseWebhookTimestamp(value); err == nil {
		return parsed.UTC()
	}
	return fallback.UTC()
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
