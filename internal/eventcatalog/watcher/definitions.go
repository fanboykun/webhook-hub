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

func Definitions() []eventcatalog.Definition {
	return []eventcatalog.Definition{
		definition(EventVersionFound, "Watcher detected a new deployable version.", []string{".Payload.version", ".Payload.service", ".Payload.watcher"}),
		definition(EventDeploymentStarted, "Watcher started a deployment attempt.", []string{".Payload.attempt", ".Payload.service", ".Payload.watcher"}),
		definition(EventDeploymentSucceeded, "Watcher completed a deployment successfully.", []string{".Payload.attempt", ".Payload.service", ".Payload.watcher"}),
		definition(EventDeploymentFailed, "Watcher reported a failed deployment attempt.", []string{".Payload.attempt", ".Payload.service", ".Payload.watcher"}),
		definition(EventDeploymentCancelled, "Watcher cancelled a deployment attempt.", []string{".Payload.attempt", ".Payload.service", ".Payload.watcher"}),
		definition(EventDeploymentRolledBack, "Watcher rolled a deployment back.", []string{".Payload.attempt", ".Payload.service", ".Payload.watcher"}),
		definition(EventRollbackSucceeded, "Watcher completed a rollback successfully.", []string{".Payload.attempt", ".Payload.service", ".Payload.watcher"}),
		definition(EventRollbackFailed, "Watcher failed to complete a rollback.", []string{".Payload.attempt", ".Payload.service", ".Payload.watcher"}),
		definition(EventWebhookTest, "Watcher emitted a webhook test event.", []string{".Payload.summary", ".Payload.watcher"}),
		definition(EventDeliveryExhausted, "Watcher reported an exhausted webhook delivery attempt.", []string{".Payload.failed_delivery", ".Payload.watcher"}),
		definition(EventServiceHealthChanged, "Watcher detected a service health transition.", []string{".Payload.health", ".Payload.service", ".Payload.watcher"}),
	}
}

func definition(key Key, summary string, templatePaths []string) eventcatalog.Definition {
	return eventcatalog.Definition{
		Key:            eventcatalog.Key(key),
		Source:         domain.SourceWatcher,
		PayloadVersion: PayloadVersionV1,
		Summary:        summary,
		TemplatePaths:  templatePaths,
		Fields: []eventcatalog.Field{
			{Path: "watcher", Type: "object", Required: true, Description: "Watcher identity and display name."},
			{Path: "summary", Type: "string", Description: "Source-provided summary when present."},
			{Path: "service", Type: "object", Description: "Service metadata when included by Watcher."},
			{Path: "version", Type: "object", Description: "Version discovery details."},
			{Path: "attempt", Type: "object", Description: "Deployment or rollback attempt details."},
			{Path: "health", Type: "object", Description: "Health transition details."},
			{Path: "failed_delivery", Type: "object", Description: "Failed downstream delivery metadata."},
		},
	}
}
