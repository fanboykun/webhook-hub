package watcher

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/iweka-dev/webhook-hub/internal/config"
	"github.com/iweka-dev/webhook-hub/internal/domain"
	"github.com/iweka-dev/webhook-hub/internal/ingress"
)

const (
	headerEventID    = "X-Watcher-Event-ID"
	headerDeliveryID = "X-Watcher-Delivery-ID"
	headerEventType  = "X-Watcher-Event"
	headerTimestamp  = "X-Watcher-Timestamp"
	headerSignature  = "X-Watcher-Signature"
)

type Adapter struct{}

type Payload struct {
	SchemaVersion any `json:"schema_version"`
	EventID       string
	EventType     string
	OccurredAt    string
	Summary       string
	TriggeredBy   string
	Watcher       struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	Version struct {
		DiscoveredVersion string `json:"discovered_version"`
		CurrentVersion    string `json:"current_version"`
		WillDeploy        bool   `json:"will_deploy"`
		BlockReason       string `json:"block_reason"`
	}
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
	}
	Service struct {
		ID             int64  `json:"id"`
		Name           string `json:"name"`
		ServiceType    string `json:"service_type"`
		HealthCheckURL string `json:"health_check_url"`
	}
	Health struct {
		PreviousStatus string `json:"previous_status"`
		CurrentStatus  string `json:"current_status"`
		HTTPStatus     int    `json:"http_status"`
		Error          string `json:"error"`
		CheckedAt      string `json:"checked_at"`
		Source         string `json:"source"`
	}
	FailedDelivery struct {
		EventID            string `json:"event_id"`
		EventType          string `json:"event_type"`
		DeliveryID         string `json:"delivery_id"`
		AttemptNumber      int    `json:"attempt_number"`
		ResponseStatusCode int    `json:"response_status_code"`
		Error              string `json:"error"`
		Summary            string `json:"summary"`
	}

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

type currentPayload struct {
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
	timestamp := req.Headers.Get(headerTimestamp)
	signature := req.Headers.Get(headerSignature)
	if timestamp == "" || signature == "" || integration.ResolvedSecret == "" {
		return ingress.ErrUnauthorized
	}

	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return ingress.ErrUnauthorized
	}
	if integration.ReplayWindow > 0 && time.Since(parsed.UTC()) > integration.ReplayWindow {
		return ingress.ErrUnauthorized
	}

	mac := hmac.New(sha256.New, []byte(integration.ResolvedSecret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte(":"))
	mac.Write(req.RawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(signature)), []byte(strings.ToLower(expected))) != 1 {
		return ingress.ErrUnauthorized
	}

	return nil
}

func (a *Adapter) Normalize(_ context.Context, integrationID string, _ config.IntegrationConfig, req ingress.InboundRequest) (ingress.AdapterResult, error) {
	var p Payload

	var current currentPayload
	currentErr := json.Unmarshal(req.RawBody, &current)

	var legacy legacyPayload
	legacyErr := json.Unmarshal(req.RawBody, &legacy)

	if currentErr != nil && legacyErr != nil {
		return ingress.AdapterResult{}, ingress.ErrMalformedPayload
	}

	if current.EventID != "" || current.EventType != "" {
		p = fromCurrentPayload(current)
	} else if legacy.ID != "" || legacy.Event != "" {
		p = fromLegacyPayload(legacy)
	} else {
		return ingress.AdapterResult{}, ingress.ErrMalformedPayload
	}

	if p.EventID == "" && p.LegacyID == "" {
		return ingress.AdapterResult{}, ingress.ErrMalformedPayload
	}

	eventType := canonicalWatcherEventType(firstNonEmpty(p.EventType, p.LegacyEvent))
	if eventType == "" {
		return ingress.AdapterResult{}, ingress.ErrMalformedPayload
	}

	occurredAt := parseTimestamp(p.OccurredAt, req.ReceivedAt)
	serviceName := firstNonEmpty(p.Service.Name, p.LegacyService)
	release := firstNonEmpty(p.Attempt.TargetVersion, p.Version.DiscoveredVersion, p.LegacyVersion)
	actor := firstNonEmpty(p.Attempt.TriggeredBy, p.TriggeredBy, p.LegacyActor)
	url := firstNonEmpty(p.LegacyURL, p.Service.HealthCheckURL)
	sourceEventID := firstNonEmpty(p.EventID, p.LegacyID)
	sourceDeliveryID := firstNonEmpty(req.Headers.Get(headerDeliveryID), req.Headers.Get(headerEventID), sourceEventID, sourceEventID+":"+eventType)

	fields := map[string]any{
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
		Severity:      watcherSeverity(eventType),
		Title:         firstNonEmpty(p.Summary, strings.ReplaceAll(eventType, ".", " ")),
		Summary:       firstNonEmpty(p.Summary, fallbackWatcherSummary(eventType, serviceName, p)),
		Service:       serviceName,
		Environment:   p.LegacyEnvironment,
		Release:       release,
		CommitSHA:     p.LegacyCommitSHA,
		Actor:         actor,
		Fingerprint:   watcherFingerprint(sourceEventID, eventType, p),
		URL:           url,
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

func fromCurrentPayload(in currentPayload) Payload {
	out := Payload{
		SchemaVersion: in.SchemaVersion,
		EventID:       in.EventID,
		EventType:     in.EventType,
		OccurredAt:    in.OccurredAt,
		Summary:       in.Summary,
		TriggeredBy:   in.TriggeredBy,
	}
	out.Watcher.ID = in.Watcher.ID
	out.Watcher.Name = in.Watcher.Name
	out.Version.DiscoveredVersion = in.Version.DiscoveredVersion
	out.Version.CurrentVersion = in.Version.CurrentVersion
	out.Version.WillDeploy = in.Version.WillDeploy
	out.Version.BlockReason = in.Version.BlockReason
	out.Attempt.ID = in.Attempt.ID
	out.Attempt.Kind = in.Attempt.Kind
	out.Attempt.Reason = in.Attempt.Reason
	out.Attempt.TriggeredBy = in.Attempt.TriggeredBy
	out.Attempt.Status = in.Attempt.Status
	out.Attempt.TargetVersion = in.Attempt.TargetVersion
	out.Attempt.FromVersion = in.Attempt.FromVersion
	out.Attempt.FailedTargetVersion = in.Attempt.FailedTargetVersion
	out.Attempt.FailurePhase = in.Attempt.FailurePhase
	out.Attempt.Error = in.Attempt.Error
	out.Attempt.ParentAttemptID = in.Attempt.ParentAttemptID
	out.Attempt.RootAttemptID = in.Attempt.RootAttemptID
	out.Service.ID = in.Service.ID
	out.Service.Name = in.Service.Name
	out.Service.ServiceType = in.Service.ServiceType
	out.Service.HealthCheckURL = in.Service.HealthCheckURL
	out.Health.PreviousStatus = in.Health.PreviousStatus
	out.Health.CurrentStatus = in.Health.CurrentStatus
	out.Health.HTTPStatus = in.Health.HTTPStatus
	out.Health.Error = in.Health.Error
	out.Health.CheckedAt = in.Health.CheckedAt
	out.Health.Source = in.Health.Source
	out.FailedDelivery.EventID = in.FailedDelivery.EventID
	out.FailedDelivery.EventType = in.FailedDelivery.EventType
	out.FailedDelivery.DeliveryID = in.FailedDelivery.DeliveryID
	out.FailedDelivery.AttemptNumber = in.FailedDelivery.AttemptNumber
	out.FailedDelivery.ResponseStatusCode = in.FailedDelivery.ResponseStatusCode
	out.FailedDelivery.Error = in.FailedDelivery.Error
	out.FailedDelivery.Summary = in.FailedDelivery.Summary
	return out
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

func canonicalWatcherEventType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "watcher.") || strings.HasPrefix(value, "service.") || strings.HasPrefix(value, "webhook.") {
		return value
	}
	return "watcher." + strings.ReplaceAll(value, "_", ".")
}

func watcherSeverity(eventType string) domain.Severity {
	switch {
	case strings.Contains(eventType, "rollback_failed"), strings.Contains(eventType, "delivery_exhausted"):
		return domain.SeverityCritical
	case strings.Contains(eventType, "failed"), strings.Contains(eventType, "unhealthy"):
		return domain.SeverityError
	case strings.Contains(eventType, "cancelled"), strings.Contains(eventType, "webhook_test"):
		return domain.SeverityWarning
	default:
		return domain.SeverityInfo
	}
}

func watcherLifecycle(eventType string) domain.Lifecycle {
	switch {
	case strings.Contains(eventType, "started"):
		return domain.LifecycleStarted
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

func watcherFingerprint(sourceEventID, eventType string, p Payload) string {
	if sourceEventID != "" {
		return sourceEventID
	}
	if p.Attempt.RootAttemptID > 0 {
		return fmt.Sprintf("%s:%d", eventType, p.Attempt.RootAttemptID)
	}
	return eventType
}

func fallbackWatcherSummary(eventType, serviceName string, p Payload) string {
	switch {
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
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC()
	}
	return fallback.UTC()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
