package httpserver

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/fanboykun/webhook-hub/internal/domain"
)

type watcherWebhookInput struct {
	IntegrationID string `path:"integration_id"`
	WebhookID     string `header:"webhook-id"`
	Timestamp     string `header:"webhook-timestamp"`
	Signature     string `header:"webhook-signature"`
	Event         string `header:"X-Watcher-Event"`
	DeliveryID    string `header:"X-Watcher-Delivery-ID"`
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

type receiptDetailInput struct {
	Authorization string `header:"Authorization" hidden:"true"`
	ReceiptID     string `path:"receipt_id"`
}

type listReceiptsInput struct {
	Authorization string             `header:"Authorization" hidden:"true"`
	Status        routeReceiptStatus `query:"status" doc:"Filter by receipt status."`
	Source        routeSource        `query:"source" doc:"Filter by source name such as watcher or github."`
	IntegrationID string             `query:"integration_id" doc:"Filter by the configured integration identifier."`
	From          string             `query:"from" doc:"Lower created_at bound in RFC3339 format." example:"2026-06-18T00:00:00Z"`
	To            string             `query:"to" doc:"Upper created_at bound in RFC3339 format." example:"2026-06-19T00:00:00Z"`
	Limit         int                `query:"limit" doc:"Maximum number of results to return. Defaults to 50 and is capped at 100." example:"50"`
	Cursor        string             `query:"cursor" doc:"Opaque pagination cursor returned by the previous response."`
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

type receiptResponse struct {
	Body struct {
		ID               string         `json:"id"`
		Source           string         `json:"source"`
		IntegrationID    string         `json:"integration_id"`
		SourceDeliveryID string         `json:"source_delivery_id"`
		SourceEventType  string         `json:"source_event_type"`
		Status           string         `json:"status"`
		IgnoreReason     string         `json:"ignore_reason,omitempty"`
		ReceivedAt       time.Time      `json:"received_at"`
		CreatedAt        time.Time      `json:"created_at"`
		Events           []receiptEvent `json:"events"`
	}
}

type receiptsResponse struct {
	Body struct {
		Items      []receiptItem `json:"items"`
		NextCursor string        `json:"next_cursor,omitempty"`
	}
}

type receiptItem struct {
	ID               string             `json:"id"`
	Source           routeSource        `json:"source"`
	IntegrationID    string             `json:"integration_id"`
	SourceDeliveryID string             `json:"source_delivery_id"`
	SourceEventType  string             `json:"source_event_type"`
	Status           routeReceiptStatus `json:"status"`
	IgnoreReason     string             `json:"ignore_reason,omitempty"`
	ReceivedAt       time.Time          `json:"received_at"`
	CreatedAt        time.Time          `json:"created_at"`
}

type receiptEvent struct {
	ID               string         `json:"id"`
	SourceEventID    string         `json:"source_event_id,omitempty"`
	Type             string         `json:"type"`
	Lifecycle        string         `json:"lifecycle"`
	Severity         string         `json:"severity"`
	Title            string         `json:"title"`
	Summary          string         `json:"summary"`
	RouteMatchCount  int            `json:"route_match_count"`
	RouteIDs         []string       `json:"route_ids,omitempty"`
	DestinationIDs   []string       `json:"destination_ids,omitempty"`
	DestinationCount int            `json:"delivery_count"`
	Deliveries       []deliveryItem `json:"deliveries"`
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
	Match        routeMatchModel    `json:"match" doc:"Different fields are ANDed together. Multiple values inside one field are ORed. Omit a field to make it unrestricted; do not send blank strings."`
	Destinations []routeDestination `json:"destinations" doc:"Destination IDs that should receive matched events."`
}

type routeMatchModel struct {
	Sources      []routeSource    `json:"sources,omitempty" doc:"Normalized source names such as watcher or github. Leave empty to match all sources."`
	Types        []routeEventType `json:"types,omitempty" doc:"Normalized event types such as watcher.deployment.failed. Leave empty to match all event types."`
	Severities   []routeSeverity  `json:"severities,omitempty" doc:"Normalized severities: debug, info, warning, error, critical. Leave empty to match any severity."`
	Environments []string         `json:"environments,omitempty" doc:"Environment values from normalized events. Leave empty to match any environment. Blank strings are rejected."`
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
		Environments: normalizeRouteStrings(in.Environments),
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
		value = routeSource(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
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
		value = routeEventType(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
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
		value = routeSeverity(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
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
		value = routeDestination(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
		out = append(out, string(value))
	}
	return out
}

func normalizeRouteStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
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

type routeReceiptStatus string

func (routeReceiptStatus) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type: "string",
		Enum: enumValues(domain.KnownReceiptStatusStrings()),
	}
}

func receiptItemFromDomain(receipt domain.Receipt) receiptItem {
	return receiptItem{
		ID:               receipt.ID,
		Source:           routeSource(receipt.Source),
		IntegrationID:    receipt.IntegrationID,
		SourceDeliveryID: receipt.SourceDeliveryID,
		SourceEventType:  receipt.SourceEventType,
		Status:           routeReceiptStatus(receipt.Status),
		IgnoreReason:     receipt.IgnoreReason,
		ReceivedAt:       receipt.ReceivedAt,
		CreatedAt:        receipt.CreatedAt,
	}
}

func receiptEventFromDomain(event domain.Event, deliveries []domain.Delivery) receiptEvent {
	routeMatchCount, routeIDs, destinationIDs := routeSummaryFromFields(event.FieldsJSON)
	out := receiptEvent{
		ID:               event.ID,
		SourceEventID:    event.SourceEventID,
		Type:             event.Type,
		Lifecycle:        string(event.Lifecycle),
		Severity:         string(event.Severity),
		Title:            event.Title,
		Summary:          event.Summary,
		RouteMatchCount:  routeMatchCount,
		RouteIDs:         routeIDs,
		DestinationIDs:   destinationIDs,
		DestinationCount: len(deliveries),
		Deliveries:       make([]deliveryItem, 0, len(deliveries)),
	}
	for _, delivery := range deliveries {
		out.Deliveries = append(out.Deliveries, deliveryItemFromDomain(delivery))
	}
	return out
}

func routeSummaryFromFields(fieldsJSON []byte) (int, []string, []string) {
	if len(fieldsJSON) == 0 {
		return 0, nil, nil
	}
	var fields map[string]any
	if err := json.Unmarshal(fieldsJSON, &fields); err != nil {
		return 0, nil, nil
	}

	routeMatchCount := 0
	if value, ok := fields["route_match_count"].(float64); ok {
		routeMatchCount = int(value)
	}
	routeIDs := stringSliceFromAny(fields["route_ids"])
	destinationIDs := stringSliceFromAny(fields["destination_ids"])
	return routeMatchCount, routeIDs, destinationIDs
}

func stringSliceFromAny(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
