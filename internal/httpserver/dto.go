package httpserver

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/fanboykun/webhook-hub/internal/domain"
	eventdefaults "github.com/fanboykun/webhook-hub/internal/eventcatalog/defaults"
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
		ReceiptID     string `json:"receipt_id" doc:"Stable receipt identifier created for this webhook delivery." example:"rcpt_01jxz6n7h2cn1n8j9h8f2y0w7c"`
		Status        string `json:"status" doc:"Final ingestion status for the stored receipt." example:"accepted"`
		Duplicate     bool   `json:"duplicate" doc:"Whether this webhook matched an already stored source delivery and was treated as a duplicate."`
		EventCount    int    `json:"event_count" doc:"Number of normalized events created from the webhook payload." example:"1"`
		DeliveryCount int    `json:"delivery_count" doc:"Number of initial delivery jobs queued from the normalized events." example:"2"`
	}
}

type healthResponse struct {
	Body map[string]string
}

type deliveryDetailInput struct {
	DeliveryID string `path:"delivery_id"`
}

type receiptDetailInput struct {
	ReceiptID string `path:"receipt_id"`
}

type listReceiptsInput struct {
	Status        routeReceiptStatus `query:"status" doc:"Filter by receipt status."`
	Source        routeSource        `query:"source" doc:"Filter by source name such as watcher or github."`
	IntegrationID string             `query:"integration_id" doc:"Filter by the configured integration identifier."`
	From          string             `query:"from" doc:"Lower created_at bound in RFC3339 format." example:"2026-06-18T00:00:00Z"`
	To            string             `query:"to" doc:"Upper created_at bound in RFC3339 format." example:"2026-06-19T00:00:00Z"`
	Limit         int                `query:"limit" doc:"Maximum number of results to return. Defaults to 50 and is capped at 100." example:"50"`
	Cursor        string             `query:"cursor" doc:"Opaque pagination cursor returned by the previous response."`
}

type listDeliveriesInput struct {
	Status        routeDeliveryStatus `query:"status" doc:"Filter by delivery status."`
	DestinationID string              `query:"destination_id" doc:"Filter by destination identifier, such as slack-deployments."`
	EventID       string              `query:"event_id" doc:"Filter by normalized event identifier."`
	From          string              `query:"from" doc:"Lower created_at bound in RFC3339 format." example:"2026-06-18T00:00:00Z"`
	To            string              `query:"to" doc:"Upper created_at bound in RFC3339 format." example:"2026-06-19T00:00:00Z"`
	Limit         int                 `query:"limit" doc:"Maximum number of results to return. Defaults to 50 and is capped at 100." example:"50"`
	Cursor        string              `query:"cursor" doc:"Opaque pagination cursor returned by the previous response."`
}

type retryDeliveryInput struct {
	DeliveryID string `path:"delivery_id"`
}

type deliveryResponse struct {
	Body struct {
		ID                string     `json:"id" doc:"Stable delivery job identifier." example:"del_01jxz6p6chm3b0zz5ff31z5j9n"`
		EventID           string     `json:"event_id" doc:"Normalized event identifier that this delivery job is sending." example:"evt_01jxz6p1zs4rmym6xg4k6m8bc2"`
		DestinationID     string     `json:"destination_id" doc:"Destination slug selected by routing, such as slack-deployments." example:"slack-deployments"`
		DestinationType   string     `json:"destination_type" doc:"Concrete sender type used for this delivery." example:"slack"`
		Status            string     `json:"status" doc:"Current delivery state." example:"retry_wait"`
		AttemptCount      int        `json:"attempt_count" doc:"How many send attempts have already been made for this delivery." example:"1"`
		MaxAttempts       int        `json:"max_attempts" doc:"Maximum send attempts before the delivery is dead-lettered." example:"5"`
		NextAttemptAt     time.Time  `json:"next_attempt_at" doc:"When the worker will next attempt this delivery."`
		LockedBy          string     `json:"locked_by,omitempty" doc:"Worker lease owner currently processing this delivery, when leased." example:"worker-1"`
		LockedUntil       *time.Time `json:"locked_until,omitempty" doc:"Lease expiration time for the current worker lock, when leased."`
		ProviderMessageID string     `json:"provider_message_id,omitempty" doc:"Provider-specific message identifier returned by the downstream sender." example:"1718905530.012300"`
		LastErrorCode     string     `json:"last_error_code,omitempty" doc:"Machine-friendly sender error code from the last failed attempt." example:"slack_http_429"`
		LastError         string     `json:"last_error,omitempty" doc:"Human-readable error message from the last failed attempt." example:"slack responded with status 429"`
		SentAt            *time.Time `json:"sent_at,omitempty" doc:"Timestamp of the successful send, when the delivery reaches sent."`
		CreatedAt         time.Time  `json:"created_at" doc:"When the delivery job was created."`
		UpdatedAt         time.Time  `json:"updated_at" doc:"When the delivery job was last updated."`
	}
}

type receiptResponse struct {
	Body struct {
		ID               string         `json:"id" doc:"Stable receipt identifier for the accepted webhook request." example:"rcpt_01jxz6n7h2cn1n8j9h8f2y0w7c"`
		Source           routeSource    `json:"source" doc:"Normalized source that verified and parsed the webhook."`
		IntegrationID    string         `json:"integration_id" doc:"Managed integration slug that accepted the webhook." example:"github-main"`
		SourceDeliveryID string         `json:"source_delivery_id" doc:"Provider delivery identifier used for deduplication, when supplied by the source." example:"5e8f2e70-31be-11ef-b4a0-2f8a7d65d12a"`
		SourceEventType  string         `json:"source_event_type" doc:"Raw provider event type received on the webhook headers or payload." example:"push"`
		Status           string         `json:"status" doc:"Final stored receipt status after verification and routing." example:"accepted"`
		IgnoreReason     string         `json:"ignore_reason,omitempty" doc:"Reason the receipt was stored but intentionally ignored, when status is ignored." example:"event type not routed"`
		ReceivedAt       time.Time      `json:"received_at" doc:"Timestamp recorded when the webhook request was accepted by the server."`
		CreatedAt        time.Time      `json:"created_at" doc:"Timestamp when the receipt row was persisted."`
		Events           []receiptEvent `json:"events" doc:"Normalized events derived from this receipt, including routing and delivery outcomes."`
	}
}

type receiptsResponse struct {
	Body struct {
		Items      []receiptItem `json:"items" doc:"Receipts in descending created_at order."`
		NextCursor string        `json:"next_cursor,omitempty" doc:"Opaque pagination cursor for the next page, if more receipts are available."`
	}
}

type receiptItem struct {
	ID               string             `json:"id" doc:"Stable receipt identifier." example:"rcpt_01jxz6n7h2cn1n8j9h8f2y0w7c"`
	Source           routeSource        `json:"source" doc:"Normalized source that accepted the webhook."`
	IntegrationID    string             `json:"integration_id" doc:"Managed integration slug that accepted the webhook." example:"watcher-production"`
	SourceDeliveryID string             `json:"source_delivery_id" doc:"Provider delivery identifier used for deduplication, when supplied by the source." example:"evt_2bR4fQ6U3vVj"`
	SourceEventType  string             `json:"source_event_type" doc:"Raw provider event type received from the source." example:"deployment.failed"`
	Status           routeReceiptStatus `json:"status" doc:"Stored receipt status."`
	IgnoreReason     string             `json:"ignore_reason,omitempty" doc:"Reason the receipt was ignored, when status is ignored." example:"event type not routed"`
	ReceivedAt       time.Time          `json:"received_at" doc:"When the webhook request was accepted by the server."`
	CreatedAt        time.Time          `json:"created_at" doc:"When the receipt row was persisted."`
}

type receiptEvent struct {
	ID               string         `json:"id" doc:"Stable normalized event identifier." example:"evt_01jxz6p1zs4rmym6xg4k6m8bc2"`
	SourceEventID    string         `json:"source_event_id,omitempty" doc:"Provider event identifier carried into the normalized event, when available." example:"1234567890"`
	Type             string         `json:"type" doc:"Normalized event type used for routing and rendering." example:"github.push"`
	Lifecycle        string         `json:"lifecycle" doc:"Normalized lifecycle classification for the event." example:"active"`
	Severity         string         `json:"severity" doc:"Normalized severity assigned to the event." example:"info"`
	Title            string         `json:"title" doc:"Short human-readable title for operators and downstream messages." example:"Push to main"`
	Summary          string         `json:"summary" doc:"Compact event summary used by renderers and audit views." example:"fanboykun pushed 3 commits to main"`
	RouteMatchCount  int            `json:"route_match_count" doc:"Number of routes whose match criteria selected this event." example:"2"`
	RouteIDs         []string       `json:"route_ids,omitempty" doc:"Route identifiers that matched this event."`
	DestinationIDs   []string       `json:"destination_ids,omitempty" doc:"Destination identifiers selected after route matches were collapsed and deduplicated."`
	DestinationCount int            `json:"delivery_count" doc:"Number of delivery jobs created for this event." example:"2"`
	Deliveries       []deliveryItem `json:"deliveries" doc:"Delivery jobs created for this event."`
}

type retryDeliveryOutput struct {
	Status int `status:"202"`
	Body   struct {
		DeliveryID string `json:"delivery_id" doc:"Delivery job identifier that was re-queued." example:"del_01jxz6p6chm3b0zz5ff31z5j9n"`
		Status     string `json:"status" doc:"Delivery status after the retry request was accepted." example:"pending"`
	}
}

type deliveriesResponse struct {
	Body struct {
		Items      []deliveryItem `json:"items" doc:"Delivery jobs in descending created_at order."`
		NextCursor string         `json:"next_cursor,omitempty" doc:"Opaque pagination cursor for the next page, if more deliveries are available."`
	}
}

type deliveryItem struct {
	ID              string              `json:"id" doc:"Stable delivery job identifier." example:"del_01jxz6p6chm3b0zz5ff31z5j9n"`
	EventID         string              `json:"event_id" doc:"Normalized event identifier being delivered." example:"evt_01jxz6p1zs4rmym6xg4k6m8bc2"`
	DestinationID   string              `json:"destination_id" doc:"Destination slug selected by routing." example:"telegram-bot"`
	DestinationType string              `json:"destination_type" doc:"Concrete sender type used for the destination." example:"telegram"`
	Status          routeDeliveryStatus `json:"status" doc:"Current delivery state."`
	AttemptCount    int                 `json:"attempt_count" doc:"How many send attempts have already been made." example:"0"`
	MaxAttempts     int                 `json:"max_attempts" doc:"Maximum send attempts before dead-lettering." example:"5"`
	NextAttemptAt   time.Time           `json:"next_attempt_at" doc:"When the worker will next attempt this delivery."`
	SentAt          *time.Time          `json:"sent_at,omitempty" doc:"Timestamp of the successful send, when the delivery reaches sent."`
	CreatedAt       time.Time           `json:"created_at" doc:"When the delivery job was created."`
	UpdatedAt       time.Time           `json:"updated_at" doc:"When the delivery job was last updated."`
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
}

type routeDetailInput struct {
	RouteID string `path:"route_id"`
}

type createRouteInput struct {
	Body routeModel
}

type updateRouteInput struct {
	RouteID string `path:"route_id"`
	Body    routeModel
}

type routeDeleteInput struct {
	RouteID string `path:"route_id"`
}

type routeResponse struct {
	Body routeModel
}

type routeDeleteOutput struct {
	Body struct {
		Deleted bool `json:"deleted"`
	}
}

type integrationModel struct {
	ID           string        `json:"id" doc:"Stable operator-defined integration identifier. This slug is used in webhook URLs, admin API paths, and persisted receipts." example:"github-main"`
	Source       routeSource   `json:"source" doc:"Webhook source adapter that verifies and normalizes incoming requests."`
	Secret       string        `json:"secret,omitempty" doc:"Redacted shared secret used to verify incoming webhook signatures. Send the real value only on create or update." example:"[REDACTED]"`
	ClientSecret string        `json:"client_secret,omitempty" doc:"Redacted secondary secret used by sources that require one, such as OAuth-backed webhook providers. Send the real value only on create or update." example:"[REDACTED]"`
	ReplayWindow time.Duration `json:"replay_window,omitempty" doc:"Allowed age for signed webhook timestamps before the request is rejected as a replay. Only supported by sources that sign timestamps, such as watcher."`
	CreatedAt    time.Time     `json:"created_at,omitempty" doc:"When the managed integration was created."`
	UpdatedAt    time.Time     `json:"updated_at,omitempty" doc:"When the managed integration was last updated."`
}

func (integrationModel) Schema(r huma.Registry) *huma.Schema {
	return unionSchema(
		r,
		"source",
		map[string]reflect.Type{
			string(domain.SourceWatcher): reflect.TypeOf(watcherIntegrationModel{}),
			string(domain.SourceGitHub):  reflect.TypeOf(githubIntegrationModel{}),
		},
	)
}

type integrationRequestModel struct {
	ID           string        `json:"id" doc:"Stable operator-defined integration identifier. Choose a slug you will use in webhook URLs and admin API paths." example:"watcher-production"`
	Source       routeSource   `json:"source" doc:"Webhook source adapter to configure."`
	Secret       string        `json:"secret,omitempty" doc:"Shared secret used to verify incoming webhook signatures." example:"whsec_live_123456"`
	ClientSecret string        `json:"client_secret,omitempty" doc:"Secondary provider secret when required by the source." example:"client_secret_live_123456"`
	ReplayWindow time.Duration `json:"replay_window,omitempty" doc:"Allowed age for signed webhook timestamps before rejecting them as replays. Only supported by sources that sign timestamps, such as watcher."`
}

func (integrationRequestModel) Schema(r huma.Registry) *huma.Schema {
	return unionSchema(
		r,
		"source",
		map[string]reflect.Type{
			string(domain.SourceWatcher): reflect.TypeOf(watcherIntegrationRequestModel{}),
			string(domain.SourceGitHub):  reflect.TypeOf(githubIntegrationRequestModel{}),
		},
	)
}

type destinationConfigModel struct {
	ID               string               `json:"id" doc:"Stable operator-defined destination identifier. Routes reference this slug and delivery records persist it." example:"slack-deployments"`
	Type             routeDestinationType `json:"type" doc:"Concrete sender type used to deliver messages."`
	WebhookURL       string               `json:"webhook_url,omitempty" doc:"Redacted Slack incoming webhook URL. Send the real value only on create or update." example:"[REDACTED]"`
	BotToken         string               `json:"bot_token,omitempty" doc:"Redacted Telegram bot token. Send the real value only on create or update." example:"[REDACTED]"`
	ChatID           string               `json:"chat_id,omitempty" doc:"Telegram chat identifier or channel username that receives rendered messages." example:"-1004353814221"`
	APIBaseURL       string               `json:"api_base_url,omitempty" doc:"Optional Telegram Bot API base URL override for self-hosted or proxied deployments." example:"https://api.telegram.org"`
	RendererProfiles []string             `json:"renderer_profiles,omitempty" doc:"Renderer profile slugs selected by this destination. At most one profile may target each source and event pair."`
	CreatedAt        time.Time            `json:"created_at,omitempty" doc:"When the managed destination was created."`
	UpdatedAt        time.Time            `json:"updated_at,omitempty" doc:"When the managed destination was last updated."`
}

func (destinationConfigModel) Schema(r huma.Registry) *huma.Schema {
	return unionSchema(
		r,
		"type",
		map[string]reflect.Type{
			string(domain.DestinationSlack):    reflect.TypeOf(slackDestinationModel{}),
			string(domain.DestinationTelegram): reflect.TypeOf(telegramDestinationModel{}),
			string(domain.DestinationTeams):    reflect.TypeOf(teamsDestinationModel{}),
		},
	)
}

type destinationRequestModel struct {
	ID               string               `json:"id" doc:"Stable operator-defined destination identifier. Choose a slug that routes and operators will reference." example:"telegram-bot"`
	Type             routeDestinationType `json:"type" doc:"Destination sender type to configure."`
	WebhookURL       string               `json:"webhook_url,omitempty" doc:"Slack incoming webhook URL used for outgoing sends." example:"https://hooks.slack.com/services/T000/B000/XXXX"`
	BotToken         string               `json:"bot_token,omitempty" doc:"Telegram bot token used for outgoing sends." example:"123456:telegram-bot-token"`
	ChatID           string               `json:"chat_id,omitempty" doc:"Telegram chat identifier or channel username that receives rendered messages." example:"-1004353814221"`
	APIBaseURL       string               `json:"api_base_url,omitempty" doc:"Optional Telegram Bot API base URL override for self-hosted or proxied deployments." example:"https://api.telegram.org"`
	RendererProfiles []string             `json:"renderer_profiles,omitempty" doc:"Renderer profile slugs selected by this destination. At most one profile may target each source and event pair."`
}

func (destinationRequestModel) Schema(r huma.Registry) *huma.Schema {
	return unionSchema(
		r,
		"type",
		map[string]reflect.Type{
			string(domain.DestinationSlack):    reflect.TypeOf(slackDestinationRequestModel{}),
			string(domain.DestinationTelegram): reflect.TypeOf(telegramDestinationRequestModel{}),
			string(domain.DestinationTeams):    reflect.TypeOf(teamsDestinationRequestModel{}),
		},
	)
}

type watcherIntegrationModel struct {
	ID           string        `json:"id" doc:"Stable operator-defined integration identifier." example:"watcher-production"`
	Source       routeSource   `json:"source" enum:"watcher" doc:"Discriminator for the Watcher webhook source."`
	Secret       string        `json:"secret,omitempty" doc:"Redacted Watcher webhook secret used for signature verification." example:"[REDACTED]"`
	ReplayWindow time.Duration `json:"replay_window,omitempty" doc:"Allowed age for Watcher webhook timestamps before they are rejected as replays."`
	CreatedAt    time.Time     `json:"created_at,omitempty" doc:"When the Watcher integration was created."`
	UpdatedAt    time.Time     `json:"updated_at,omitempty" doc:"When the Watcher integration was last updated."`
}

type githubIntegrationModel struct {
	ID        string      `json:"id" doc:"Stable operator-defined integration identifier." example:"github-main"`
	Source    routeSource `json:"source" enum:"github" doc:"Discriminator for the GitHub webhook source."`
	Secret    string      `json:"secret,omitempty" doc:"Redacted GitHub webhook secret used for X-Hub-Signature-256 verification." example:"[REDACTED]"`
	CreatedAt time.Time   `json:"created_at,omitempty" doc:"When the GitHub integration was created."`
	UpdatedAt time.Time   `json:"updated_at,omitempty" doc:"When the GitHub integration was last updated."`
}

type watcherIntegrationRequestModel struct {
	ID           string        `json:"id" doc:"Stable operator-defined integration identifier." example:"watcher-production"`
	Source       routeSource   `json:"source" enum:"watcher" doc:"Discriminator for the Watcher webhook source."`
	Secret       string        `json:"secret,omitempty" doc:"Watcher webhook secret used for signature verification." example:"whsec_live_123456"`
	ReplayWindow time.Duration `json:"replay_window,omitempty" doc:"Allowed age for Watcher webhook timestamps before they are rejected as replays."`
}

type githubIntegrationRequestModel struct {
	ID     string      `json:"id" doc:"Stable operator-defined integration identifier." example:"github-main"`
	Source routeSource `json:"source" enum:"github" doc:"Discriminator for the GitHub webhook source."`
	Secret string      `json:"secret,omitempty" doc:"GitHub webhook secret used for X-Hub-Signature-256 verification." example:"github_webhook_secret"`
}

type slackDestinationModel struct {
	ID               string               `json:"id" doc:"Stable operator-defined destination identifier." example:"slack-deployments"`
	Type             routeDestinationType `json:"type" enum:"slack" doc:"Discriminator for the Slack sender."`
	WebhookURL       string               `json:"webhook_url,omitempty" doc:"Redacted Slack incoming webhook URL used for outgoing sends." example:"[REDACTED]"`
	RendererProfiles []string             `json:"renderer_profiles,omitempty" doc:"Event-specific renderer profiles selected before sending to Slack."`
	CreatedAt        time.Time            `json:"created_at,omitempty" doc:"When the Slack destination was created."`
	UpdatedAt        time.Time            `json:"updated_at,omitempty" doc:"When the Slack destination was last updated."`
}

type telegramDestinationModel struct {
	ID               string               `json:"id" doc:"Stable operator-defined destination identifier." example:"telegram-bot"`
	Type             routeDestinationType `json:"type" enum:"telegram" doc:"Discriminator for the Telegram sender."`
	BotToken         string               `json:"bot_token,omitempty" doc:"Redacted Telegram bot token used for outgoing sends." example:"[REDACTED]"`
	ChatID           string               `json:"chat_id,omitempty" doc:"Telegram chat identifier or channel username that receives messages." example:"-1004353814221"`
	APIBaseURL       string               `json:"api_base_url,omitempty" doc:"Optional Telegram Bot API base URL override." example:"https://api.telegram.org"`
	RendererProfiles []string             `json:"renderer_profiles,omitempty" doc:"Event-specific renderer profiles selected before sending to Telegram."`
	CreatedAt        time.Time            `json:"created_at,omitempty" doc:"When the Telegram destination was created."`
	UpdatedAt        time.Time            `json:"updated_at,omitempty" doc:"When the Telegram destination was last updated."`
}

type teamsDestinationModel struct {
	ID               string               `json:"id" doc:"Stable operator-defined destination identifier." example:"teams-oncall"`
	Type             routeDestinationType `json:"type" enum:"teams" doc:"Discriminator for the Microsoft Teams webhook sender."`
	WebhookURL       string               `json:"webhook_url,omitempty" doc:"Redacted Microsoft Teams incoming webhook URL used for outgoing sends." example:"[REDACTED]"`
	RendererProfiles []string             `json:"renderer_profiles,omitempty" doc:"Event-specific renderer profiles selected before sending to Teams."`
	CreatedAt        time.Time            `json:"created_at,omitempty" doc:"When the Teams destination was created."`
	UpdatedAt        time.Time            `json:"updated_at,omitempty" doc:"When the Teams destination was last updated."`
}

type slackDestinationRequestModel struct {
	ID               string               `json:"id" doc:"Stable operator-defined destination identifier." example:"slack-deployments"`
	Type             routeDestinationType `json:"type" enum:"slack" doc:"Discriminator for the Slack sender."`
	WebhookURL       string               `json:"webhook_url,omitempty" doc:"Slack incoming webhook URL used for outgoing sends." example:"https://hooks.slack.com/services/T000/B000/XXXX"`
	RendererProfiles []string             `json:"renderer_profiles,omitempty" doc:"Event-specific renderer profiles selected before sending to Slack."`
}

type telegramDestinationRequestModel struct {
	ID               string               `json:"id" doc:"Stable operator-defined destination identifier." example:"telegram-bot"`
	Type             routeDestinationType `json:"type" enum:"telegram" doc:"Discriminator for the Telegram sender."`
	BotToken         string               `json:"bot_token,omitempty" doc:"Telegram bot token used for outgoing sends." example:"123456:telegram-bot-token"`
	ChatID           string               `json:"chat_id,omitempty" doc:"Telegram chat identifier or channel username that receives messages." example:"-1004353814221"`
	APIBaseURL       string               `json:"api_base_url,omitempty" doc:"Optional Telegram Bot API base URL override." example:"https://api.telegram.org"`
	RendererProfiles []string             `json:"renderer_profiles,omitempty" doc:"Event-specific renderer profiles selected before sending to Telegram."`
}

type listIntegrationsInput struct {
	Source routeSource `query:"source" doc:"Filter by integration source such as watcher or github."`
}

type integrationDetailInput struct {
	IntegrationID string `path:"integration_id" doc:"Stable integration identifier, for example github-main or watcher-production." example:"github-main"`
}

type createIntegrationInput struct {
	Body integrationRequestModel
}

type updateIntegrationInput struct {
	IntegrationID string `path:"integration_id" doc:"Stable integration identifier, for example github-main or watcher-production." example:"github-main"`
	Body          integrationRequestModel
}

type deleteIntegrationInput struct {
	IntegrationID string `path:"integration_id" doc:"Stable integration identifier, for example github-main or watcher-production." example:"github-main"`
}

type integrationsResponse struct {
	Body struct {
		Items []integrationModel `json:"items" doc:"Managed webhook integrations."`
	}
}

type integrationResponse struct {
	Body integrationModel
}

type teamsDestinationRequestModel struct {
	ID               string               `json:"id" doc:"Stable operator-defined destination identifier." example:"teams-oncall"`
	Type             routeDestinationType `json:"type" enum:"teams" doc:"Discriminator for the Microsoft Teams webhook sender."`
	WebhookURL       string               `json:"webhook_url,omitempty" doc:"Microsoft Teams incoming webhook URL used for outgoing sends." example:"https://example.webhook.office.com/webhookb2/..."`
	RendererProfiles []string             `json:"renderer_profiles,omitempty" doc:"Event-specific renderer profiles selected before sending to Teams."`
}

type listDestinationsInput struct {
	Type routeDestinationType `query:"type" doc:"Filter by destination type such as slack, telegram, or teams."`
}

type destinationDetailInput struct {
	DestinationID string `path:"destination_id" doc:"Stable destination identifier, for example slack-deployments or telegram-bot." example:"slack-deployments"`
}

type createDestinationInput struct {
	Body destinationRequestModel
}

type updateDestinationInput struct {
	DestinationID string `path:"destination_id" doc:"Stable destination identifier, for example slack-deployments or telegram-bot." example:"slack-deployments"`
	Body          destinationRequestModel
}

type deleteDestinationInput struct {
	DestinationID string `path:"destination_id" doc:"Stable destination identifier, for example slack-deployments or telegram-bot." example:"slack-deployments"`
}

type destinationsResponse struct {
	Body struct {
		Items []destinationConfigModel `json:"items" doc:"Managed delivery destinations."`
	}
}

type destinationResponse struct {
	Body destinationConfigModel
}

type rendererProfileModel struct {
	ID        string                 `json:"id" doc:"Stable renderer profile identifier referenced by destinations." example:"watcher-deployment-failed-detailed"`
	Event     rendererEventRefModel  `json:"event" doc:"Exact source event contract this profile targets."`
	Templates rendererTemplatesModel `json:"templates" doc:"Destination templates for this specific event type."`
	CreatedAt time.Time              `json:"created_at,omitempty" doc:"When the renderer profile was created."`
	UpdatedAt time.Time              `json:"updated_at,omitempty" doc:"When the renderer profile was last updated."`
}

type rendererProfileRequestModel struct {
	ID        string                 `json:"id" doc:"Stable renderer profile identifier referenced by destinations." example:"watcher-deployment-failed-compact"`
	Event     rendererEventRefModel  `json:"event" doc:"Exact source event contract this profile targets."`
	Templates rendererTemplatesModel `json:"templates" doc:"Destination templates for this specific event type."`
}

type rendererEventRefModel struct {
	Source routeSource    `json:"source" doc:"Normalized source that owns this event contract."`
	Key    routeEventType `json:"key" doc:"Normalized event key within that source catalog."`
}

type rendererTemplatesModel struct {
	Slack    *rendererSlackTemplateModel    `json:"slack,omitempty" doc:"Optional Slack template pair for this scope."`
	Telegram *rendererTelegramTemplateModel `json:"telegram,omitempty" doc:"Optional Telegram template for this scope."`
	Teams    *rendererTeamsTemplateModel    `json:"teams,omitempty" doc:"Optional Microsoft Teams template pair for this scope."`
	Email    *rendererEmailTemplateModel    `json:"email,omitempty" doc:"Optional email template pair for this scope."`
}

type rendererSlackTemplateModel struct {
	Title string `json:"title,omitempty" doc:"Go template for the Slack top-level text field used in notifications and previews." example:"[{{.Severity}}] {{.Title}}"`
	Body  string `json:"body,omitempty" doc:"Go template for the main Slack mrkdwn body block." example:"*{{.Title}}*\n{{.Summary}}"`
}

type rendererTelegramTemplateModel struct {
	Text string `json:"text,omitempty" doc:"Go template for the Telegram HTML message body." example:"<b>{{.Title}}</b>\n{{.Summary}}"`
}

type rendererTeamsTemplateModel struct {
	Title string `json:"title,omitempty" doc:"Go template for the Microsoft Teams card title." example:"[{{.Severity}}] {{.Title}}"`
	Body  string `json:"body,omitempty" doc:"Go template for the Microsoft Teams card body." example:"**{{.Title}}**\n\n{{.Summary}}"`
}

type rendererEmailTemplateModel struct {
	Subject string `json:"subject,omitempty" doc:"Go template for the email subject line." example:"[{{.Severity}}] {{.Title}}"`
	Body    string `json:"body,omitempty" doc:"Go template for the email body." example:"{{.Summary}}"`
}

type listRendererProfilesInput struct {
}

type rendererProfileDetailInput struct {
	ProfileID string `path:"profile_id" doc:"Stable renderer profile identifier, for example watcher-deployment-failed-detailed." example:"watcher-deployment-failed-detailed"`
}

type createRendererProfileInput struct {
	Body rendererProfileRequestModel
}

type updateRendererProfileInput struct {
	ProfileID string `path:"profile_id" doc:"Stable renderer profile identifier, for example watcher-deployment-failed-detailed." example:"watcher-deployment-failed-detailed"`
	Body      rendererProfileRequestModel
}

type deleteRendererProfileInput struct {
	ProfileID string `path:"profile_id" doc:"Stable renderer profile identifier, for example watcher-deployment-failed-detailed." example:"watcher-deployment-failed-detailed"`
}

type rendererProfilesResponse struct {
	Body struct {
		Items []rendererProfileModel `json:"items" doc:"Managed renderer profiles that destinations can reference."`
	}
}

type rendererProfileResponse struct {
	Body rendererProfileModel
}

type deleteEntityOutput struct {
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

func integrationModelFromDomain(in domain.ManagedIntegration) integrationModel {
	return integrationModel{
		ID:           in.ID,
		Source:       routeSource(in.Source),
		Secret:       "[REDACTED]",
		ClientSecret: redactIfPresent(in.ClientSecret),
		ReplayWindow: in.ReplayWindow,
		CreatedAt:    in.CreatedAt,
		UpdatedAt:    in.UpdatedAt,
	}
}

func domainIntegrationFromModel(in integrationModel) domain.ManagedIntegration {
	return domain.ManagedIntegration{
		ID:           in.ID,
		Source:       domain.Source(in.Source),
		Secret:       in.Secret,
		ClientSecret: in.ClientSecret,
		ReplayWindow: in.ReplayWindow,
	}
}

func domainIntegrationFromRequestModel(in integrationRequestModel) domain.ManagedIntegration {
	return domain.ManagedIntegration{
		ID:           in.ID,
		Source:       domain.Source(in.Source),
		Secret:       in.Secret,
		ClientSecret: in.ClientSecret,
		ReplayWindow: in.ReplayWindow,
	}
}

func destinationModelFromDomain(in domain.ManagedDestination) destinationConfigModel {
	return destinationConfigModel{
		ID:               in.ID,
		Type:             routeDestinationType(in.Type),
		WebhookURL:       redactIfPresent(in.WebhookURL),
		BotToken:         redactIfPresent(in.BotToken),
		ChatID:           in.ChatID,
		APIBaseURL:       in.APIBaseURL,
		RendererProfiles: append([]string(nil), in.RendererProfiles...),
		CreatedAt:        in.CreatedAt,
		UpdatedAt:        in.UpdatedAt,
	}
}

func domainDestinationFromModel(in destinationConfigModel) domain.ManagedDestination {
	return domain.ManagedDestination{
		ID:               in.ID,
		Type:             domain.DestinationType(in.Type),
		WebhookURL:       in.WebhookURL,
		BotToken:         in.BotToken,
		ChatID:           in.ChatID,
		APIBaseURL:       in.APIBaseURL,
		RendererProfiles: append([]string(nil), in.RendererProfiles...),
	}
}

func domainDestinationFromRequestModel(in destinationRequestModel) domain.ManagedDestination {
	return domain.ManagedDestination{
		ID:               in.ID,
		Type:             domain.DestinationType(in.Type),
		WebhookURL:       in.WebhookURL,
		BotToken:         in.BotToken,
		ChatID:           in.ChatID,
		APIBaseURL:       in.APIBaseURL,
		RendererProfiles: append([]string(nil), in.RendererProfiles...),
	}
}

func rendererProfileModelFromDomain(in domain.ManagedRendererProfile) rendererProfileModel {
	return rendererProfileModel{
		ID:        in.ID,
		Event:     rendererEventRefModel{Source: routeSource(in.Profile.Source), Key: routeEventType(in.Profile.Key)},
		Templates: rendererTemplatesModelFromDomain(in.Profile.Templates),
		CreatedAt: in.CreatedAt,
		UpdatedAt: in.UpdatedAt,
	}
}

func domainRendererProfileFromRequestModel(in rendererProfileRequestModel) (domain.ManagedRendererProfile, error) {
	source := domain.Source(strings.TrimSpace(string(in.Event.Source)))
	eventKey := strings.TrimSpace(string(in.Event.Key))
	return domain.ManagedRendererProfile{
		ID: in.ID,
		Profile: domain.RendererProfile{
			Source:    source,
			Key:       eventKey,
			Templates: rendererTemplatesModelToDomain(in.Templates),
		},
	}, nil
}

func rendererTemplatesModelFromDomain(in domain.RendererDestinationTemplates) rendererTemplatesModel {
	out := rendererTemplatesModel{}
	if in.Slack != nil {
		out.Slack = &rendererSlackTemplateModel{
			Title: in.Slack.Title,
			Body:  in.Slack.Body,
		}
	}
	if in.Telegram != nil {
		out.Telegram = &rendererTelegramTemplateModel{Text: in.Telegram.Text}
	}
	if in.Teams != nil {
		out.Teams = &rendererTeamsTemplateModel{
			Title: in.Teams.Title,
			Body:  in.Teams.Body,
		}
	}
	if in.Email != nil {
		out.Email = &rendererEmailTemplateModel{
			Subject: in.Email.Subject,
			Body:    in.Email.Body,
		}
	}
	return out
}

func rendererTemplatesModelToDomain(in rendererTemplatesModel) domain.RendererDestinationTemplates {
	out := domain.RendererDestinationTemplates{}
	if in.Slack != nil {
		out.Slack = &domain.SlackTemplate{
			Title: in.Slack.Title,
			Body:  in.Slack.Body,
		}
	}
	if in.Telegram != nil {
		out.Telegram = &domain.TelegramTemplate{Text: in.Telegram.Text}
	}
	if in.Teams != nil {
		out.Teams = &domain.TeamsTemplate{
			Title: in.Teams.Title,
			Body:  in.Teams.Body,
		}
	}
	if in.Email != nil {
		out.Email = &domain.EmailTemplate{
			Subject: in.Email.Subject,
			Body:    in.Email.Body,
		}
	}
	return out
}

func redactIfPresent(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "[REDACTED]"
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
		Type:        "string",
		Description: "Normalized webhook source identifier.",
		Enum:        enumValues(domain.KnownSourceStrings()),
	}
}

type routeSeverity string

func (routeSeverity) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:        "string",
		Description: "Normalized event severity.",
		Enum:        enumValues(domain.KnownSeverityStrings()),
	}
}

type routeEventType string

func (routeEventType) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:        "string",
		Description: "Normalized event type used for routing.",
		Enum:        enumValues(eventdefaults.Registry().EventTypes()),
	}
}

type routeDestination string

func (routeDestination) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:        "string",
		Description: "Stable destination identifier referenced by routes.",
	}
}

type routeDestinationType string

func (routeDestinationType) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:        "string",
		Description: "Concrete sender type used for a destination.",
		Enum: enumValues([]string{
			string(domain.DestinationSlack),
			string(domain.DestinationTelegram),
			string(domain.DestinationTeams),
		}),
	}
}

func unionSchema(r huma.Registry, discriminatorProperty string, variants map[string]reflect.Type) *huma.Schema {
	oneOf := make([]*huma.Schema, 0, len(variants))
	mapping := make(map[string]string, len(variants))
	for key, variantType := range variants {
		schema := r.Schema(variantType, true, variantType.Name())
		oneOf = append(oneOf, schema)
		if schema.Ref != "" {
			mapping[key] = schema.Ref
		}
	}
	return &huma.Schema{
		OneOf: oneOf,
		Discriminator: &huma.Discriminator{
			PropertyName: discriminatorProperty,
			Mapping:      mapping,
		},
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
		Type:        "string",
		Description: "Current delivery job state.",
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
		Type:        "string",
		Description: "Stored webhook receipt status.",
		Enum:        enumValues(domain.KnownReceiptStatusStrings()),
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
	routeMatchCount, routeIDs, destinationIDs := routeSummaryFromTrace(event.RouteTraceJSON)
	out := receiptEvent{
		ID:               event.ID,
		SourceEventID:    event.SourceEventID,
		Type:             event.Key,
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

func routeSummaryFromTrace(traceJSON []byte) (int, []string, []string) {
	if len(traceJSON) == 0 {
		return 0, nil, nil
	}
	var trace domain.RouteTrace
	if err := json.Unmarshal(traceJSON, &trace); err != nil {
		return 0, nil, nil
	}
	return trace.RouteMatchCount, trace.RouteIDs, trace.DestinationIDs
}
