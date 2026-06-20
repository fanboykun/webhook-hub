package sqlite

import "time"

type receiptModel struct {
	ID               string `gorm:"primaryKey"`
	Source           string `gorm:"index:idx_receipt_source_delivery,unique"`
	IntegrationID    string `gorm:"index:idx_receipt_source_delivery,unique"`
	SourceDeliveryID string `gorm:"index:idx_receipt_source_delivery,unique"`
	SourceEventType  string
	PayloadSHA256    string
	RawPayload       []byte
	HeadersJSON      []byte
	ReceivedAt       time.Time
	Status           string
	IgnoreReason     string
	CreatedAt        time.Time
}

type eventModel struct {
	ID            string `gorm:"primaryKey"`
	ReceiptID     string `gorm:"index"`
	Source        string `gorm:"index"`
	IntegrationID string
	SourceEventID string
	Type          string `gorm:"index"`
	Action        string
	Lifecycle     string
	Severity      string
	Title         string
	Summary       string
	Service       string `gorm:"index:idx_events_service_env_occurred"`
	Environment   string `gorm:"index:idx_events_service_env_occurred"`
	Release       string
	CommitSHA     string
	Actor         string
	Fingerprint   string `gorm:"index"`
	GroupKey      string
	URL           string
	OccurredAt    time.Time `gorm:"index:idx_events_service_env_occurred"`
	LabelsJSON    []byte
	FieldsJSON    []byte
	CreatedAt     time.Time
}

type deliveryModel struct {
	ID                string `gorm:"primaryKey"`
	EventID           string `gorm:"index:idx_delivery_event_destination,unique"`
	DestinationID     string `gorm:"index:idx_delivery_event_destination,unique"`
	DestinationType   string
	Status            string `gorm:"index:idx_deliveries_status_next_attempt"`
	AttemptCount      int
	MaxAttempts       int
	NextAttemptAt     time.Time `gorm:"index:idx_deliveries_status_next_attempt"`
	LockedBy          string
	LockedUntil       *time.Time
	ProviderMessageID string
	LastErrorCode     string
	LastError         string
	SentAt            *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type deliveryAttemptModel struct {
	ID            string `gorm:"primaryKey"`
	DeliveryID    string `gorm:"index:idx_delivery_attempt_sequence,unique"`
	AttemptNumber int    `gorm:"index:idx_delivery_attempt_sequence,unique"`
	WorkerID      string
	StartedAt     time.Time
	CompletedAt   time.Time
	Outcome       string
	ResponseCode  int
	ErrorCode     string
	ErrorMessage  string
	DurationMS    int64
}

type routeModel struct {
	ID               string `gorm:"primaryKey"`
	Description      string
	MatchJSON        []byte
	DestinationsJSON []byte
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type integrationModel struct {
	ID               string `gorm:"primaryKey"`
	Source           string `gorm:"index"`
	ConfigCiphertext []byte
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (integrationModel) TableName() string {
	return "integrations"
}

type destinationModel struct {
	ID               string `gorm:"primaryKey"`
	Type             string `gorm:"index"`
	ConfigCiphertext []byte
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (destinationModel) TableName() string {
	return "destinations"
}

type rendererProfileModel struct {
	ID          string `gorm:"primaryKey"`
	ProfileJSON []byte
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (rendererProfileModel) TableName() string {
	return "renderer_profiles"
}
