package httpserver

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/iweka-dev/webhook-hub/internal/config"
)

func registerRoutes(api huma.API, cfg config.Config, handler *Handler) {
	huma.Register(api, huma.Operation{
		OperationID:   "watcher-webhook",
		Method:        http.MethodPost,
		Path:          "/webhooks/v1/watcher/{integration_id}",
		Summary:       "Receive Watcher webhook events",
		Description:   "Accepts a signed Watcher webhook, verifies the HMAC signature against the raw request body, normalizes the provider payload into one or more internal events, persists the receipt and delivery jobs, and returns `202 Accepted` only after the work is durable.",
		Tags:          []string{"Watcher"},
		DefaultStatus: http.StatusAccepted,
		MaxBodyBytes:  cfg.Server.MaxWebhookBodyBytes,
	}, handler.handleWatcherWebhook)

	huma.Register(api, huma.Operation{
		OperationID:   "github-webhook",
		Method:        http.MethodPost,
		Path:          "/webhooks/v1/github/{integration_id}",
		Summary:       "Receive GitHub webhook events",
		Description:   "Accepts a signed GitHub webhook, verifies `X-Hub-Signature-256` using the raw request body, normalizes supported event families such as pull requests, workflow runs, and releases, persists the receipt and delivery jobs, and returns `202 Accepted` only after the write commits.",
		Tags:          []string{"GitHub"},
		DefaultStatus: http.StatusAccepted,
		MaxBodyBytes:  cfg.Server.MaxWebhookBodyBytes,
	}, handler.handleGitHubWebhook)

	huma.Register(api, huma.Operation{
		OperationID: "health-live",
		Method:      http.MethodGet,
		Path:        "/health/live",
		Summary:     "Process liveness check",
		Description: "Returns success while the process is running and able to serve HTTP traffic. This endpoint does not check database connectivity or downstream providers.",
		Tags:        []string{"Health"},
	}, handler.healthLive)

	huma.Register(api, huma.Operation{
		OperationID: "health-ready",
		Method:      http.MethodGet,
		Path:        "/health/ready",
		Summary:     "Database and worker readiness check",
		Description: "Returns success only when the gateway has completed startup requirements needed to accept durable work, including database connectivity and application initialization. Temporary provider outages should not normally make this endpoint fail.",
		Tags:        []string{"Health"},
	}, handler.healthReady)

	huma.Register(api, huma.Operation{
		OperationID: "deliveries-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/deliveries",
		Summary:     "List deliveries",
		Description: "Returns persisted delivery jobs with optional filters for status, destination, event, and creation time window. Results are ordered newest-first and support cursor pagination through `next_cursor`.",
		Tags:        []string{"Deliveries"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.listDeliveries)

	huma.Register(api, huma.Operation{
		OperationID: "delivery-detail",
		Method:      http.MethodGet,
		Path:        "/api/v1/deliveries/{delivery_id}",
		Summary:     "Get delivery detail",
		Description: "Returns one persisted delivery record, including its current state, retry counters, provider identifiers, last error fields, and timestamps useful for operator diagnosis.",
		Tags:        []string{"Deliveries"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.getDelivery)

	huma.Register(api, huma.Operation{
		OperationID:   "delivery-retry",
		Method:        http.MethodPost,
		Path:          "/api/v1/deliveries/{delivery_id}/retry",
		Summary:       "Retry a delivery",
		Description:   "Moves an eligible failed delivery back to the retry queue without deleting attempt history. Use this after resolving a destination or configuration problem that caused the original failure.",
		Tags:          []string{"Deliveries"},
		Security:      []map[string][]string{{"bearerAuth": {}}},
		DefaultStatus: http.StatusAccepted,
	}, handler.retryDelivery)

	huma.Register(api, huma.Operation{
		OperationID: "routes-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/routes",
		Summary:     "List routes",
		Description: "Returns the current persisted routing rules that the in-memory routing engine evaluates for normalized events. Each rule is additive, and duplicate destination matches are collapsed during delivery creation.",
		Tags:        []string{"Routes"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.listRoutes)

	huma.Register(api, huma.Operation{
		OperationID: "route-detail",
		Method:      http.MethodGet,
		Path:        "/api/v1/routes/{route_id}",
		Summary:     "Get route detail",
		Description: "Returns one persisted routing rule by identifier, including its match criteria and destination fan-out targets.",
		Tags:        []string{"Routes"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.getRoute)

	huma.Register(api, huma.Operation{
		OperationID: "route-create",
		Method:      http.MethodPost,
		Path:        "/api/v1/routes",
		Summary:     "Create route",
		Description: "Creates a new persisted routing rule. The application validates referenced destinations, stores the rule durably, and reloads the in-memory routing engine after the write succeeds.",
		Tags:        []string{"Routes"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.createRoute)

	huma.Register(api, huma.Operation{
		OperationID: "route-update",
		Method:      http.MethodPut,
		Path:        "/api/v1/routes/{route_id}",
		Summary:     "Update route",
		Description: "Replaces the stored match criteria and destination targets for one routing rule, then refreshes the active in-memory routing engine so new events start using the updated policy immediately.",
		Tags:        []string{"Routes"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.updateRoute)

	huma.Register(api, huma.Operation{
		OperationID: "route-delete",
		Method:      http.MethodDelete,
		Path:        "/api/v1/routes/{route_id}",
		Summary:     "Delete route",
		Description: "Deletes one persisted routing rule and reloads the active in-memory routing engine so future events no longer evaluate against it.",
		Tags:        []string{"Routes"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.deleteRoute)
}
