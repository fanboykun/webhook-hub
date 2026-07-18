package watcher

import (
	"time"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/eventcatalog"
)

const (
	EventVersionFound         Key = "watcher.version.found"
	EventDeploymentStarted    Key = "watcher.deployment.started"
	EventDeploymentSucceeded  Key = "watcher.deployment.succeeded"
	EventDeploymentFailed     Key = "watcher.deployment.failed"
	EventDeploymentCancelled  Key = "watcher.deployment.cancelled"
	EventDeploymentRolledBack Key = "watcher.deployment.rolled_back"
	EventRollbackSucceeded    Key = "watcher.rollback.succeeded"
	EventRollbackFailed       Key = "watcher.rollback.failed"
	EventWebhookTest          Key = "watcher.webhook.test"
	EventDeliveryExhausted    Key = "webhook.delivery.exhausted"
	EventServiceHealthChanged Key = "service.health.changed"
	PayloadVersionV1              = 1
)

type Key string

type WatcherRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type ServiceDetails struct {
	ID             int64  `json:"id,omitempty"`
	Name           string `json:"name,omitempty"`
	ServiceType    string `json:"service_type,omitempty"`
	HealthCheckURL string `json:"health_check_url,omitempty"`
}

type VersionDetails struct {
	DiscoveredVersion string `json:"discovered_version,omitempty"`
	CurrentVersion    string `json:"current_version,omitempty"`
	WillDeploy        bool   `json:"will_deploy,omitempty"`
	BlockReason       string `json:"block_reason,omitempty"`
}

type AttemptDetails struct {
	ID                  int64  `json:"id,omitempty"`
	Kind                string `json:"kind,omitempty"`
	Reason              string `json:"reason,omitempty"`
	TriggeredBy         string `json:"triggered_by,omitempty"`
	Status              string `json:"status,omitempty"`
	TargetVersion       string `json:"target_version,omitempty"`
	FromVersion         string `json:"from_version,omitempty"`
	FailedTargetVersion string `json:"failed_target_version,omitempty"`
	FailurePhase        string `json:"failure_phase,omitempty"`
	Error               string `json:"error,omitempty"`
	ParentAttemptID     *int64 `json:"parent_attempt_id,omitempty"`
	RootAttemptID       int64  `json:"root_attempt_id,omitempty"`
}

type HealthDetails struct {
	PreviousStatus string `json:"previous_status,omitempty"`
	CurrentStatus  string `json:"current_status,omitempty"`
	HTTPStatus     int    `json:"http_status,omitempty"`
	Error          string `json:"error,omitempty"`
	CheckedAt      string `json:"checked_at,omitempty"`
	Source         string `json:"source,omitempty"`
}

type FailedDeliveryDetails struct {
	EventID            string `json:"event_id,omitempty"`
	EventType          string `json:"event_type,omitempty"`
	DeliveryID         string `json:"delivery_id,omitempty"`
	AttemptNumber      int    `json:"attempt_number,omitempty"`
	ResponseStatusCode int    `json:"response_status_code,omitempty"`
	Error              string `json:"error,omitempty"`
	Summary            string `json:"summary,omitempty"`
}

type Payload struct {
	Summary        string                 `json:"summary,omitempty"`
	Watcher        WatcherRef             `json:"watcher"`
	Service        *ServiceDetails        `json:"service,omitempty"`
	Version        *VersionDetails        `json:"version,omitempty"`
	Attempt        *AttemptDetails        `json:"attempt,omitempty"`
	Health         *HealthDetails         `json:"health,omitempty"`
	FailedDelivery *FailedDeliveryDetails `json:"failed_delivery,omitempty"`
}

type ProjectionInput struct {
	IntegrationID     string
	NormalizedType    Key
	OccurredAt        time.Time
	SourceEventID     string
	SourceDeliveryID  string
	Summary           string
	ServiceName       string
	Environment       string
	Release           string
	CommitSHA         string
	Actor             string
	URL               string
	Labels            map[string]string
	SchemaVersion     any
	TriggeredBy       string
	Watcher           WatcherRef
	Service           *ServiceDetails
	Version           *VersionDetails
	Attempt           *AttemptDetails
	Health            *HealthDetails
	FailedDelivery    *FailedDeliveryDetails
	LegacyErrorStage  string
	LegacyErrorReason string
}

func IsKnown(key Key) bool {
	switch key {
	case EventVersionFound, EventDeploymentStarted, EventDeploymentSucceeded, EventDeploymentFailed,
		EventDeploymentCancelled, EventDeploymentRolledBack, EventRollbackSucceeded, EventRollbackFailed,
		EventWebhookTest, EventDeliveryExhausted, EventServiceHealthChanged:
		return true
	default:
		return false
	}
}

func Definitions() []eventcatalog.Definition {
	return []eventcatalog.Definition{
		definition(EventVersionFound, "Watcher detected a new deployable version.", versionFields()),
		definition(EventDeploymentStarted, "Watcher started a deployment attempt.", attemptFields()),
		definition(EventDeploymentSucceeded, "Watcher completed a deployment successfully.", attemptFields()),
		definition(EventDeploymentFailed, "Watcher reported a failed deployment attempt.", attemptFields()),
		definition(EventDeploymentCancelled, "Watcher cancelled a deployment attempt.", attemptFields()),
		definition(EventDeploymentRolledBack, "Watcher rolled a deployment back.", attemptFields()),
		definition(EventRollbackSucceeded, "Watcher completed a rollback successfully.", attemptFields()),
		definition(EventRollbackFailed, "Watcher failed to complete a rollback.", attemptFields()),
		definition(EventWebhookTest, "Watcher emitted a webhook test event.", baseFields()),
		definition(EventDeliveryExhausted, "Watcher reported an exhausted webhook delivery attempt.", failedDeliveryFields()),
		definition(EventServiceHealthChanged, "Watcher detected a service health transition.", healthFields()),
	}
}

func definition(key Key, summary string, fields []eventcatalog.Field) eventcatalog.Definition {
	return eventcatalog.Definition{
		Key:            eventcatalog.Key(key),
		Source:         domain.SourceWatcher,
		PayloadVersion: PayloadVersionV1,
		Summary:        summary,
		Fields:         fields,
	}
}

func baseFields() []eventcatalog.Field {
	return []eventcatalog.Field{
		{Path: "watcher", Type: eventcatalog.FieldTypeObject, Required: true, Description: "Watcher identity and display name."},
		{Path: "watcher.id", Type: eventcatalog.FieldTypeNumber, Description: "Watcher numeric identifier."},
		{Path: "watcher.name", Type: eventcatalog.FieldTypeString, Description: "Watcher display name."},
		{Path: "summary", Type: eventcatalog.FieldTypeString, Description: "Source-provided summary when present."},
	}
}

func serviceFields() []eventcatalog.Field {
	return []eventcatalog.Field{
		{Path: "service", Type: eventcatalog.FieldTypeObject, Required: true, Description: "Service metadata."},
		{Path: "service.id", Type: eventcatalog.FieldTypeNumber, Description: "Watcher service identifier."},
		{Path: "service.name", Type: eventcatalog.FieldTypeString, Description: "Watcher service name."},
		{Path: "service.service_type", Type: eventcatalog.FieldTypeString, Description: "Watcher service type."},
		{Path: "service.health_check_url", Type: eventcatalog.FieldTypeString, Description: "Service health check URL."},
	}
}

func versionFields() []eventcatalog.Field {
	return append(append(baseFields(), serviceFields()...),
		eventcatalog.Field{Path: "version", Type: eventcatalog.FieldTypeObject, Required: true, Description: "Version discovery details."},
		eventcatalog.Field{Path: "version.discovered_version", Type: eventcatalog.FieldTypeString, Description: "Discovered deployable version."},
		eventcatalog.Field{Path: "version.current_version", Type: eventcatalog.FieldTypeString, Description: "Currently deployed version."},
		eventcatalog.Field{Path: "version.will_deploy", Type: eventcatalog.FieldTypeBoolean, Description: "Whether Watcher will deploy the discovered version."},
		eventcatalog.Field{Path: "version.block_reason", Type: eventcatalog.FieldTypeString, Description: "Reason a discovered version will not deploy."},
	)
}

func attemptFields() []eventcatalog.Field {
	return append(append(baseFields(), serviceFields()...),
		eventcatalog.Field{Path: "attempt", Type: eventcatalog.FieldTypeObject, Required: true, Description: "Deployment or rollback attempt details."},
		eventcatalog.Field{Path: "attempt.id", Type: eventcatalog.FieldTypeNumber, Description: "Attempt identifier."},
		eventcatalog.Field{Path: "attempt.kind", Type: eventcatalog.FieldTypeString, Description: "Attempt kind."},
		eventcatalog.Field{Path: "attempt.reason", Type: eventcatalog.FieldTypeString, Description: "Attempt reason."},
		eventcatalog.Field{Path: "attempt.triggered_by", Type: eventcatalog.FieldTypeString, Description: "Actor or system that triggered the attempt."},
		eventcatalog.Field{Path: "attempt.status", Type: eventcatalog.FieldTypeString, Description: "Attempt status."},
		eventcatalog.Field{Path: "attempt.target_version", Type: eventcatalog.FieldTypeString, Description: "Target deploy version."},
		eventcatalog.Field{Path: "attempt.from_version", Type: eventcatalog.FieldTypeString, Description: "Previously deployed version."},
		eventcatalog.Field{Path: "attempt.failed_target_version", Type: eventcatalog.FieldTypeString, Description: "Failed target version."},
		eventcatalog.Field{Path: "attempt.failure_phase", Type: eventcatalog.FieldTypeString, Description: "Failure phase."},
		eventcatalog.Field{Path: "attempt.error", Type: eventcatalog.FieldTypeString, Description: "Attempt error details."},
		eventcatalog.Field{Path: "attempt.parent_attempt_id", Type: eventcatalog.FieldTypeNumber, Description: "Parent attempt identifier."},
		eventcatalog.Field{Path: "attempt.root_attempt_id", Type: eventcatalog.FieldTypeNumber, Description: "Root attempt identifier."},
	)
}

func healthFields() []eventcatalog.Field {
	return append(append(baseFields(), serviceFields()...),
		eventcatalog.Field{Path: "health", Type: eventcatalog.FieldTypeObject, Required: true, Description: "Health transition details."},
		eventcatalog.Field{Path: "health.previous_status", Type: eventcatalog.FieldTypeString, Description: "Previous health status."},
		eventcatalog.Field{Path: "health.current_status", Type: eventcatalog.FieldTypeString, Description: "Current health status."},
		eventcatalog.Field{Path: "health.http_status", Type: eventcatalog.FieldTypeNumber, Description: "HTTP status observed during health check."},
		eventcatalog.Field{Path: "health.error", Type: eventcatalog.FieldTypeString, Description: "Health error details."},
		eventcatalog.Field{Path: "health.checked_at", Type: eventcatalog.FieldTypeString, Description: "Health check timestamp."},
		eventcatalog.Field{Path: "health.source", Type: eventcatalog.FieldTypeString, Description: "Health check source."},
	)
}

func failedDeliveryFields() []eventcatalog.Field {
	return append(baseFields(),
		eventcatalog.Field{Path: "failed_delivery", Type: eventcatalog.FieldTypeObject, Required: true, Description: "Failed downstream delivery metadata."},
		eventcatalog.Field{Path: "failed_delivery.event_id", Type: eventcatalog.FieldTypeString, Description: "Original failed event identifier."},
		eventcatalog.Field{Path: "failed_delivery.event_type", Type: eventcatalog.FieldTypeString, Description: "Original failed event type."},
		eventcatalog.Field{Path: "failed_delivery.delivery_id", Type: eventcatalog.FieldTypeString, Description: "Failed delivery identifier."},
		eventcatalog.Field{Path: "failed_delivery.attempt_number", Type: eventcatalog.FieldTypeNumber, Description: "Failed delivery attempt number."},
		eventcatalog.Field{Path: "failed_delivery.response_status_code", Type: eventcatalog.FieldTypeNumber, Description: "Provider response status."},
		eventcatalog.Field{Path: "failed_delivery.error", Type: eventcatalog.FieldTypeString, Description: "Provider error text."},
		eventcatalog.Field{Path: "failed_delivery.summary", Type: eventcatalog.FieldTypeString, Description: "Failed delivery summary."},
	)
}
