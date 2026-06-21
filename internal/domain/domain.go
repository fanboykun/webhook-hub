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
	LifecycleUpdated    Lifecycle = "updated"
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

type EventScope struct {
	Service     string
	Environment string
}

type EventEnvelope struct {
	Source         Source
	IntegrationID  string
	SourceEventID  string
	Key            string
	Action         string
	Lifecycle      Lifecycle
	Severity       Severity
	Title          string
	Summary        string
	Scope          EventScope
	Fingerprint    string
	GroupKey       string
	SourceURL      string
	OccurredAt     time.Time
	LabelsJSON     []byte
	MetadataJSON   []byte
	PayloadVersion int
	PayloadJSON    []byte
}

type EventCandidate struct {
	EventEnvelope
}

type Event struct {
	ID        string
	ReceiptID string
	EventEnvelope
	RouteTraceJSON []byte
	CreatedAt      time.Time
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

type RouteTrace struct {
	RouteMatchCount int      `json:"route_match_count"`
	RouteIDs        []string `json:"route_ids,omitempty"`
	DestinationIDs  []string `json:"destination_ids,omitempty"`
}

type IngestResult struct {
	ReceiptID     string
	Duplicate     bool
	EventCount    int
	DeliveryCount int
	Status        ReceiptStatus
}

type ReceiptDetail struct {
	Receipt    Receipt
	Events     []Event
	Deliveries map[string][]Delivery
}

type ReceiptFilter struct {
	Status        ReceiptStatus
	Source        Source
	IntegrationID string
	From          *time.Time
	To            *time.Time
	Limit         int
	Cursor        string
}

type ReceiptPage struct {
	Items      []Receipt
	NextCursor string
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

type ManagedIntegration struct {
	ID           string
	Source       Source
	Secret       string
	ClientSecret string
	ReplayWindow time.Duration
	CreatedAt    time.Time
	UpdatedAt    time.Time
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

var KnownReceiptStatuses = []ReceiptStatus{
	ReceiptAccepted,
	ReceiptIgnored,
	ReceiptDuplicate,
	ReceiptUnrouted,
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

func KnownReceiptStatusStrings() []string {
	values := make([]string, 0, len(KnownReceiptStatuses))
	for _, value := range KnownReceiptStatuses {
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
