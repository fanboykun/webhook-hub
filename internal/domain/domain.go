package domain

import "time"

type Source string

const (
	SourceWatcher Source = "watcher"
	SourceGitHub  Source = "github"
	SourceGrafana Source = "grafana"
	SourceSentry  Source = "sentry"
)

type Lifecycle string

const (
	LifecycleStarted    Lifecycle = "started"
	LifecycleTriggered  Lifecycle = "triggered"
	LifecycleSucceeded  Lifecycle = "succeeded"
	LifecycleFailed     Lifecycle = "failed"
	LifecycleResolved   Lifecycle = "resolved"
	LifecycleCancelled  Lifecycle = "cancelled"
	LifecycleRolledBack Lifecycle = "rolled_back"
)

type Severity string

const (
	SeverityDebug    Severity = "debug"
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

type DestinationType string

const (
	DestinationSlack    DestinationType = "slack"
	DestinationTelegram DestinationType = "telegram"
	DestinationEmail    DestinationType = "email"
)

type ReceiptStatus string

const (
	ReceiptAccepted  ReceiptStatus = "accepted"
	ReceiptIgnored   ReceiptStatus = "ignored"
	ReceiptDuplicate ReceiptStatus = "duplicate"
	ReceiptUnrouted  ReceiptStatus = "unrouted"
)

type DeliveryStatus string

const (
	DeliveryPending    DeliveryStatus = "pending"
	DeliveryProcessing DeliveryStatus = "processing"
	DeliveryRetryWait  DeliveryStatus = "retry_wait"
	DeliverySent       DeliveryStatus = "sent"
	DeliveryDeadLetter DeliveryStatus = "dead_letter"
)

type Receipt struct {
	ID               string
	Source           Source
	IntegrationID    string
	SourceDeliveryID string
	SourceEventType  string
	PayloadSHA256    string
	RawPayload       []byte
	HeadersJSON      []byte
	ReceivedAt       time.Time
	Status           ReceiptStatus
	IgnoreReason     string
	CreatedAt        time.Time
}

type Event struct {
	ID            string
	ReceiptID     string
	Source        Source
	IntegrationID string
	SourceEventID string
	Type          string
	Action        string
	Lifecycle     Lifecycle
	Severity      Severity
	Title         string
	Summary       string
	Service       string
	Environment   string
	Release       string
	CommitSHA     string
	Actor         string
	Fingerprint   string
	GroupKey      string
	URL           string
	OccurredAt    time.Time
	LabelsJSON    []byte
	FieldsJSON    []byte
	CreatedAt     time.Time
}

type Delivery struct {
	ID                string
	EventID           string
	DestinationID     string
	DestinationType   DestinationType
	Status            DeliveryStatus
	AttemptCount      int
	MaxAttempts       int
	NextAttemptAt     time.Time
	LockedBy          string
	LockedUntil       *time.Time
	ProviderMessageID string
	LastErrorCode     string
	LastError         string
	SentAt            *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type DeliveryAttempt struct {
	ID            string
	DeliveryID    string
	AttemptNumber int
	WorkerID      string
	StartedAt     time.Time
	CompletedAt   time.Time
	Outcome       string
	ResponseCode  int
	ErrorCode     string
	ErrorMessage  string
	DurationMS    int64
}

type RouteMatch struct {
	RouteID       string
	DestinationID string
}

type Route struct {
	ID           string
	Description  string
	Match        RouteMatchCriteria
	Destinations []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type RouteMatchCriteria struct {
	Sources      []Source
	Types        []string
	Severities   []Severity
	Environments []string
}

type IngestBatch struct {
	Receipt         Receipt
	Events          []Event
	DeliveryByEvent map[string][]Delivery
}

type IngestResult struct {
	ReceiptID     string
	Duplicate     bool
	EventCount    int
	DeliveryCount int
	Status        ReceiptStatus
}

type ClaimRequest struct {
	WorkerID      string
	BatchSize     int
	LeaseDuration time.Duration
	Now           time.Time
}

type DeliveryEnvelope struct {
	Delivery Delivery
	Event    Event
}

type DeliveryFilter struct {
	Status        DeliveryStatus
	DestinationID string
	EventID       string
	From          *time.Time
	To            *time.Time
	Limit         int
	Cursor        string
}

type DeliveryPage struct {
	Items      []Delivery
	NextCursor string
}

type RenderedMessage struct {
	ContentType string
	Body        []byte
}

type Destination struct {
	ID      string
	Type    DestinationType
	Profile string
}

type AttemptResult struct {
	DeliveryID        string
	WorkerID          string
	StartedAt         time.Time
	CompletedAt       time.Time
	Outcome           string
	ResponseCode      int
	ErrorCode         string
	ErrorMessage      string
	ProviderMessageID string
	NextStatus        DeliveryStatus
	NextAttemptAt     *time.Time
}

var KnownEventTypes = []string{
	"watcher.deployment.started",
	"watcher.deployment.succeeded",
	"watcher.deployment.failed",
	"watcher.deployment.cancelled",
	"watcher.deployment.rolled_back",
	"github.pull_request.opened",
	"github.pull_request.merged",
	"github.pull_request.closed",
	"github.workflow.succeeded",
	"github.workflow.failed",
	"github.workflow.cancelled",
	"github.release.published",
}

var KnownSources = []Source{
	SourceWatcher,
	SourceGitHub,
	SourceGrafana,
	SourceSentry,
}

var KnownSeverities = []Severity{
	SeverityDebug,
	SeverityInfo,
	SeverityWarning,
	SeverityError,
	SeverityCritical,
}

func KnownSourceStrings() []string {
	values := make([]string, 0, len(KnownSources))
	for _, value := range KnownSources {
		values = append(values, string(value))
	}
	return values
}

func IsKnownSource(value Source) bool {
	for _, known := range KnownSources {
		if known == value {
			return true
		}
	}
	return false
}

func KnownSeverityStrings() []string {
	values := make([]string, 0, len(KnownSeverities))
	for _, value := range KnownSeverities {
		values = append(values, string(value))
	}
	return values
}

func IsKnownSeverity(value Severity) bool {
	for _, known := range KnownSeverities {
		if known == value {
			return true
		}
	}
	return false
}

func KnownEventTypeStrings() []string {
	values := make([]string, 0, len(KnownEventTypes))
	for _, value := range KnownEventTypes {
		values = append(values, value)
	}
	return values
}

func IsKnownEventType(value string) bool {
	for _, known := range KnownEventTypes {
		if known == value {
			return true
		}
	}
	return false
}
