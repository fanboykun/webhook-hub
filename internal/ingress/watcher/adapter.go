package watcher

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	watchercatalog "github.com/fanboykun/webhook-hub/internal/eventcatalog/watcher"
	"github.com/fanboykun/webhook-hub/internal/ingress"
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
	if !watchercatalog.IsKnown(watchercatalog.Key(eventType)) {
		return ingress.AdapterResult{
			SourceDeliveryID: sourceDeliveryID,
			SourceEventType:  eventType,
			IgnoreReason:     "watcher event is not supported",
		}, nil
	}

	input := watchercatalog.ProjectionInput{
		IntegrationID:     integrationID,
		NormalizedType:    watchercatalog.Key(eventType),
		OccurredAt:        occurredAt,
		SourceEventID:     sourceEventID,
		SourceDeliveryID:  sourceDeliveryID,
		Summary:           p.Summary,
		ServiceName:       serviceName,
		Environment:       p.LegacyEnvironment,
		Release:           release,
		CommitSHA:         p.LegacyCommitSHA,
		Actor:             actor,
		URL:               firstNonEmpty(url, p.Service.HealthCheckURL),
		Labels:            p.LegacyLabels,
		SchemaVersion:     p.SchemaVersion,
		TriggeredBy:       p.TriggeredBy,
		Watcher:           watchercatalog.WatcherRef{ID: p.Watcher.ID, Name: p.Watcher.Name},
		LegacyErrorStage:  p.LegacyError.Stage,
		LegacyErrorReason: p.LegacyError.Message,
	}
	if service := watcherServiceDetails(p); service != nil {
		input.Service = service
	} else if p.LegacyService != "" {
		input.Service = &watchercatalog.ServiceDetails{Name: p.LegacyService}
	}
	if version := watcherVersionDetails(p); version != nil {
		input.Version = version
	}
	if attempt := watcherAttemptDetails(p); attempt != nil {
		input.Attempt = attempt
	} else if p.LegacyVersion != "" || p.LegacyError.Stage != "" || p.LegacyError.Message != "" {
		input.Attempt = &watchercatalog.AttemptDetails{
			TargetVersion: p.LegacyVersion,
			FailurePhase:  p.LegacyError.Stage,
			Error:         p.LegacyError.Message,
		}
	}
	if health := watcherHealthDetails(p); health != nil {
		input.Health = health
	}
	if failedDelivery := watcherFailedDeliveryDetails(p); failedDelivery != nil {
		input.FailedDelivery = failedDelivery
	}
	event, err := watchercatalog.Project(input)
	if err != nil {
		return ingress.AdapterResult{}, err
	}

	return ingress.AdapterResult{
		SourceDeliveryID: sourceDeliveryID,
		SourceEventType:  eventType,
		Events:           []domain.EventCandidate{event},
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

func watcherServiceDetails(p Payload) *watchercatalog.ServiceDetails {
	if p.Service.ID == 0 && p.Service.Name == "" && p.Service.ServiceType == "" && p.Service.HealthCheckURL == "" {
		return nil
	}
	return &watchercatalog.ServiceDetails{
		ID:             p.Service.ID,
		Name:           p.Service.Name,
		ServiceType:    p.Service.ServiceType,
		HealthCheckURL: p.Service.HealthCheckURL,
	}
}

func watcherVersionDetails(p Payload) *watchercatalog.VersionDetails {
	if p.Version.DiscoveredVersion == "" && p.Version.CurrentVersion == "" && p.Version.BlockReason == "" && !p.Version.WillDeploy {
		return nil
	}
	return &watchercatalog.VersionDetails{
		DiscoveredVersion: p.Version.DiscoveredVersion,
		CurrentVersion:    p.Version.CurrentVersion,
		WillDeploy:        p.Version.WillDeploy,
		BlockReason:       p.Version.BlockReason,
	}
}

func watcherAttemptDetails(p Payload) *watchercatalog.AttemptDetails {
	if p.Attempt.ID == 0 && p.Attempt.Kind == "" && p.Attempt.Status == "" {
		return nil
	}
	return &watchercatalog.AttemptDetails{
		ID:                  p.Attempt.ID,
		Kind:                p.Attempt.Kind,
		Reason:              p.Attempt.Reason,
		TriggeredBy:         p.Attempt.TriggeredBy,
		Status:              p.Attempt.Status,
		TargetVersion:       p.Attempt.TargetVersion,
		FromVersion:         p.Attempt.FromVersion,
		FailedTargetVersion: p.Attempt.FailedTargetVersion,
		FailurePhase:        p.Attempt.FailurePhase,
		Error:               p.Attempt.Error,
		ParentAttemptID:     p.Attempt.ParentAttemptID,
		RootAttemptID:       p.Attempt.RootAttemptID,
	}
}

func watcherHealthDetails(p Payload) *watchercatalog.HealthDetails {
	if p.Health.PreviousStatus == "" && p.Health.CurrentStatus == "" && p.Health.HTTPStatus == 0 && p.Health.Error == "" {
		return nil
	}
	return &watchercatalog.HealthDetails{
		PreviousStatus: p.Health.PreviousStatus,
		CurrentStatus:  p.Health.CurrentStatus,
		HTTPStatus:     p.Health.HTTPStatus,
		Error:          p.Health.Error,
		CheckedAt:      p.Health.CheckedAt,
		Source:         p.Health.Source,
	}
}

func watcherFailedDeliveryDetails(p Payload) *watchercatalog.FailedDeliveryDetails {
	if p.FailedDelivery.EventID == "" && p.FailedDelivery.DeliveryID == "" {
		return nil
	}
	return &watchercatalog.FailedDeliveryDetails{
		EventID:            p.FailedDelivery.EventID,
		EventType:          p.FailedDelivery.EventType,
		DeliveryID:         p.FailedDelivery.DeliveryID,
		AttemptNumber:      p.FailedDelivery.AttemptNumber,
		ResponseStatusCode: p.FailedDelivery.ResponseStatusCode,
		Error:              p.FailedDelivery.Error,
		Summary:            p.FailedDelivery.Summary,
	}
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
