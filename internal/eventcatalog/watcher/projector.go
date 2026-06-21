package watcher

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fanboykun/webhook-hub/internal/domain"
)

func Project(input ProjectionInput) (domain.EventCandidate, error) {
	payload := Payload{
		Summary: input.Summary,
		Watcher: input.Watcher,
		Service: input.Service,
		Version: input.Version,
		Attempt: input.Attempt,
		Health:  input.Health,
	}
	if input.FailedDelivery != nil {
		payload.FailedDelivery = input.FailedDelivery
	}

	labelsJSON, _ := json.Marshal(input.Labels)
	metadataJSON := mustJSON(map[string]any{
		"schema_version": input.SchemaVersion,
		"triggered_by":   input.TriggeredBy,
		"actor":          input.Actor,
		"release":        input.Release,
		"commit_sha":     input.CommitSHA,
	})
	if input.LegacyErrorReason != "" || input.LegacyErrorStage != "" {
		metadata := map[string]any{
			"schema_version":     input.SchemaVersion,
			"triggered_by":       input.TriggeredBy,
			"actor":              input.Actor,
			"release":            input.Release,
			"commit_sha":         input.CommitSHA,
			"legacy_error_stage": input.LegacyErrorStage,
			"legacy_error":       input.LegacyErrorReason,
		}
		metadataJSON = mustJSON(metadata)
	}

	return domain.EventCandidate{
		EventEnvelope: domain.EventEnvelope{
			Source:        domain.SourceWatcher,
			IntegrationID: input.IntegrationID,
			SourceEventID: input.SourceEventID,
			Key:           string(input.NormalizedType),
			Action:        action(input.NormalizedType),
			Lifecycle:     lifecycle(input.NormalizedType),
			Severity:      severity(input.NormalizedType, input),
			Title:         firstNonEmpty(input.Summary, fallbackTitle(input.NormalizedType, input)),
			Summary:       firstNonEmpty(input.Summary, fallbackSummary(input.NormalizedType, input.ServiceName)),
			Scope: domain.EventScope{
				Service:     input.ServiceName,
				Environment: input.Environment,
			},
			Fingerprint:    fingerprint(input),
			SourceURL:      input.URL,
			OccurredAt:     input.OccurredAt,
			LabelsJSON:     labelsJSON,
			MetadataJSON:   metadataJSON,
			PayloadVersion: PayloadVersionV1,
			PayloadJSON:    mustJSON(payload),
		},
	}, nil
}

func action(key Key) string {
	switch key {
	case EventVersionFound:
		return "version.found"
	case EventDeploymentStarted:
		return "deployment.started"
	case EventDeploymentSucceeded:
		return "deployment.succeeded"
	case EventDeploymentFailed:
		return "deployment.failed"
	case EventDeploymentCancelled:
		return "deployment.cancelled"
	case EventDeploymentRolledBack:
		return "deployment.rolled_back"
	case EventRollbackSucceeded:
		return "rollback.succeeded"
	case EventRollbackFailed:
		return "rollback.failed"
	case EventWebhookTest:
		return "webhook.test"
	case EventDeliveryExhausted:
		return "delivery.exhausted"
	case EventServiceHealthChanged:
		return "health.changed"
	default:
		return string(key)
	}
}

func lifecycle(key Key) domain.Lifecycle {
	switch key {
	case EventDeploymentStarted:
		return domain.LifecycleStarted
	case EventDeploymentSucceeded, EventRollbackSucceeded:
		return domain.LifecycleSucceeded
	case EventDeploymentFailed, EventRollbackFailed, EventDeliveryExhausted:
		return domain.LifecycleFailed
	case EventDeploymentCancelled:
		return domain.LifecycleCancelled
	case EventDeploymentRolledBack:
		return domain.LifecycleRolledBack
	default:
		return domain.LifecycleUpdated
	}
}

func severity(key Key, input ProjectionInput) domain.Severity {
	switch key {
	case EventDeploymentFailed, EventRollbackFailed, EventDeliveryExhausted:
		return domain.SeverityError
	case EventServiceHealthChanged:
		if input.Health != nil && strings.EqualFold(input.Health.CurrentStatus, "healthy") {
			return domain.SeverityInfo
		}
		return domain.SeverityWarning
	default:
		return domain.SeverityInfo
	}
}

func fallbackTitle(key Key, input ProjectionInput) string {
	switch key {
	case EventVersionFound:
		return fmt.Sprintf("New version found for %s", input.ServiceName)
	case EventDeploymentFailed:
		return fmt.Sprintf("Deployment failed for %s", input.ServiceName)
	case EventDeploymentSucceeded:
		return fmt.Sprintf("Deployment succeeded for %s", input.ServiceName)
	case EventDeploymentStarted:
		return fmt.Sprintf("Deployment started for %s", input.ServiceName)
	case EventDeliveryExhausted:
		if input.FailedDelivery != nil {
			return fmt.Sprintf("Webhook delivery exhausted: %s", input.FailedDelivery.EventType)
		}
	}
	return fmt.Sprintf("%s %s", input.ServiceName, action(key))
}

func fallbackSummary(key Key, serviceName string) string {
	return fmt.Sprintf("%s %s", serviceName, action(key))
}

func fingerprint(input ProjectionInput) string {
	return fmt.Sprintf("%s:%s", input.SourceEventID, input.NormalizedType)
}

func mustJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
