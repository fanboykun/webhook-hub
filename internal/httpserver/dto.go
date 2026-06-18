package httpserver

import (
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/iweka-dev/webhook-hub/internal/domain"
)

type watcherWebhookInput struct {
	IntegrationID string `path:"integration_id"`
	Event         string `header:"X-Watcher-Event"`
	EventID       string `header:"X-Watcher-Event-ID"`
	DeliveryID    string `header:"X-Watcher-Delivery-ID"`
	Timestamp     string `header:"X-Watcher-Timestamp"`
	Signature     string `header:"X-Watcher-Signature"`
	RawBody       []byte
}

type githubWebhookInput struct {
	IntegrationID string `path:"integration_id"`
	Event         string `header:"X-GitHub-Event"`
	DeliveryID    string `header:"X-GitHub-Delivery"`
	Signature     string `header:"X-Hub-Signature-256"`
	RawBody       []byte
}

type watcherWebhookOutput struct {
	Status int `status:"202"`
	Body   struct {
		ReceiptID     string `json:"receipt_id"`
		Status        string `json:"status"`
		Duplicate     bool   `json:"duplicate"`
		EventCount    int    `json:"event_count"`
		DeliveryCount int    `json:"delivery_count"`
	}
}

type healthResponse struct {
	Body map[string]string
}

type deliveryDetailInput struct {
	Authorization string `header:"Authorization" hidden:"true"`
	DeliveryID    string `path:"delivery_id"`
}

type listDeliveriesInput struct {
	Authorization string              `header:"Authorization" hidden:"true"`
	Status        routeDeliveryStatus `query:"status" doc:"Filter by delivery status."`
	DestinationID string              `query:"destination_id" doc:"Filter by destination identifier, such as slack-deployments."`
	EventID       string              `query:"event_id" doc:"Filter by normalized event identifier."`
	From          string              `query:"from" doc:"Lower created_at bound in RFC3339 format." example:"2026-06-18T00:00:00Z"`
	To            string              `query:"to" doc:"Upper created_at bound in RFC3339 format." example:"2026-06-19T00:00:00Z"`
	Limit         int                 `query:"limit" doc:"Maximum number of results to return. Defaults to 50 and is capped at 100." example:"50"`
	Cursor        string              `query:"cursor" doc:"Opaque pagination cursor returned by the previous response."`
}

type retryDeliveryInput struct {
	Authorization string `header:"Authorization" hidden:"true"`
	DeliveryID    string `path:"delivery_id"`
}

type deliveryResponse struct {
	Body struct {
		ID                string     `json:"id"`
		EventID           string     `json:"event_id"`
		DestinationID     string     `json:"destination_id"`
		DestinationType   string     `json:"destination_type"`
		Status            string     `json:"status"`
		AttemptCount      int        `json:"attempt_count"`
		MaxAttempts       int        `json:"max_attempts"`
		NextAttemptAt     time.Time  `json:"next_attempt_at"`
		LockedBy          string     `json:"locked_by,omitempty"`
		LockedUntil       *time.Time `json:"locked_until,omitempty"`
		ProviderMessageID string     `json:"provider_message_id,omitempty"`
		LastErrorCode     string     `json:"last_error_code,omitempty"`
		LastError         string     `json:"last_error,omitempty"`
		SentAt            *time.Time `json:"sent_at,omitempty"`
		CreatedAt         time.Time  `json:"created_at"`
		UpdatedAt         time.Time  `json:"updated_at"`
	}
}

type retryDeliveryOutput struct {
	Status int `status:"202"`
	Body   struct {
		DeliveryID string `json:"delivery_id"`
		Status     string `json:"status"`
	}
}

type deliveriesResponse struct {
	Body struct {
		Items      []deliveryItem `json:"items"`
		NextCursor string         `json:"next_cursor,omitempty"`
	}
}

type deliveryItem struct {
	ID              string              `json:"id"`
	EventID         string              `json:"event_id"`
	DestinationID   string              `json:"destination_id"`
	DestinationType string              `json:"destination_type"`
	Status          routeDeliveryStatus `json:"status"`
	AttemptCount    int                 `json:"attempt_count"`
	MaxAttempts     int                 `json:"max_attempts"`
	NextAttemptAt   time.Time           `json:"next_attempt_at"`
	SentAt          *time.Time          `json:"sent_at,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

type routesResponse struct {
	Body struct {
		Routes []routeModel `json:"routes"`
	}
}

type routeModel struct {
	ID           string             `json:"id" doc:"Stable unique route identifier." example:"watcher-all-events"`
	Description  string             `json:"description,omitempty" doc:"Optional operator-facing note about why this route exists." example:"Send all Watcher events to Telegram bot"`
	Match        routeMatchModel    `json:"match" doc:"Different fields are ANDed together. Multiple values inside one field are ORed."`
	Destinations []routeDestination `json:"destinations" doc:"Destination IDs that should receive matched events."`
}

type routeMatchModel struct {
	Sources      []routeSource    `json:"sources,omitempty" doc:"Normalized source names such as watcher or github."`
	Types        []routeEventType `json:"types,omitempty" doc:"Normalized event types such as watcher.deployment.failed."`
	Severities   []routeSeverity  `json:"severities,omitempty" doc:"Normalized severities: debug, info, warning, error, critical."`
	Environments []string         `json:"environments,omitempty" doc:"Environment values from normalized events."`
}

type listRoutesInput struct {
	Authorization string `header:"Authorization" hidden:"true"`
}

type routeDetailInput struct {
	Authorization string `header:"Authorization" hidden:"true"`
	RouteID       string `path:"route_id"`
}

type createRouteInput struct {
	Authorization string `header:"Authorization" hidden:"true"`
	Body          routeModel
}

type updateRouteInput struct {
	Authorization string `header:"Authorization" hidden:"true"`
	RouteID       string `path:"route_id"`
	Body          routeModel
}

type routeDeleteInput struct {
	Authorization string `header:"Authorization" hidden:"true"`
	RouteID       string `path:"route_id"`
}

type routeResponse struct {
	Body routeModel
}

type routeDeleteOutput struct {
	Body struct {
		Deleted bool `json:"deleted"`
	}
}

func routeModelFromDomain(route domain.Route) routeModel {
	return routeModel{
		ID:           route.ID,
		Description:  route.Description,
		Match:        routeMatchModelFromDomain(route.Match),
		Destinations: routeDestinationsFromStrings(route.Destinations),
	}
}

func routeModelsFromDomain(routes []domain.Route) []routeModel {
	out := make([]routeModel, 0, len(routes))
	for _, route := range routes {
		out = append(out, routeModelFromDomain(route))
	}
	return out
}

func domainRouteFromModel(route routeModel) domain.Route {
	return domain.Route{
		ID:           route.ID,
		Description:  route.Description,
		Match:        domainRouteMatchFromModel(route.Match),
		Destinations: routeDestinationsToStrings(route.Destinations),
	}
}

func routeMatchModelFromDomain(in domain.RouteMatchCriteria) routeMatchModel {
	return routeMatchModel{
		Sources:      routeSourcesFromDomain(in.Sources),
		Types:        routeEventTypesFromStrings(in.Types),
		Severities:   routeSeveritiesFromDomain(in.Severities),
		Environments: append([]string(nil), in.Environments...),
	}
}

func domainRouteMatchFromModel(in routeMatchModel) domain.RouteMatchCriteria {
	return domain.RouteMatchCriteria{
		Sources:      routeSourcesToDomain(in.Sources),
		Types:        routeEventTypesToStrings(in.Types),
		Severities:   routeSeveritiesToDomain(in.Severities),
		Environments: append([]string(nil), in.Environments...),
	}
}

func routeSourcesFromDomain(values []domain.Source) []routeSource {
	out := make([]routeSource, 0, len(values))
	for _, value := range values {
		out = append(out, routeSource(value))
	}
	return out
}

func routeSourcesToDomain(values []routeSource) []domain.Source {
	out := make([]domain.Source, 0, len(values))
	for _, value := range values {
		out = append(out, domain.Source(value))
	}
	return out
}

func routeEventTypesFromStrings(values []string) []routeEventType {
	out := make([]routeEventType, 0, len(values))
	for _, value := range values {
		out = append(out, routeEventType(value))
	}
	return out
}

func routeEventTypesToStrings(values []routeEventType) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func routeSeveritiesFromDomain(values []domain.Severity) []routeSeverity {
	out := make([]routeSeverity, 0, len(values))
	for _, value := range values {
		out = append(out, routeSeverity(value))
	}
	return out
}

func routeSeveritiesToDomain(values []routeSeverity) []domain.Severity {
	out := make([]domain.Severity, 0, len(values))
	for _, value := range values {
		out = append(out, domain.Severity(value))
	}
	return out
}

func routeDestinationsFromStrings(values []string) []routeDestination {
	out := make([]routeDestination, 0, len(values))
	for _, value := range values {
		out = append(out, routeDestination(value))
	}
	return out
}

func routeDestinationsToStrings(values []routeDestination) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

type routeSource string

func (routeSource) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type: "string",
		Enum: enumValues(domain.KnownSourceStrings()),
	}
}

type routeSeverity string

func (routeSeverity) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type: "string",
		Enum: enumValues(domain.KnownSeverityStrings()),
	}
}

type routeEventType string

func (routeEventType) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{Type: "string", Enum: enumValues(domain.KnownEventTypeStrings())}
}

type routeDestination string

func (routeDestination) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type: "string",
	}
}

func enumValues(values []string) []any {
	enum := make([]any, 0, len(values))
	for _, value := range values {
		enum = append(enum, value)
	}
	return enum
}

type routeDeliveryStatus string

func (routeDeliveryStatus) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type: "string",
		Enum: enumValues([]string{
			string(domain.DeliveryPending),
			string(domain.DeliveryProcessing),
			string(domain.DeliveryRetryWait),
			string(domain.DeliverySent),
			string(domain.DeliveryDeadLetter),
		}),
	}
}

func deliveryItemFromDomain(delivery domain.Delivery) deliveryItem {
	return deliveryItem{
		ID:              delivery.ID,
		EventID:         delivery.EventID,
		DestinationID:   delivery.DestinationID,
		DestinationType: string(delivery.DestinationType),
		Status:          routeDeliveryStatus(delivery.Status),
		AttemptCount:    delivery.AttemptCount,
		MaxAttempts:     delivery.MaxAttempts,
		NextAttemptAt:   delivery.NextAttemptAt,
		SentAt:          delivery.SentAt,
		CreatedAt:       delivery.CreatedAt,
		UpdatedAt:       delivery.UpdatedAt,
	}
}
