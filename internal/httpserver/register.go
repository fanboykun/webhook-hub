package httpserver

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/fanboykun/webhook-hub/internal/config"
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
		OperationID: "receipt-detail",
		Method:      http.MethodGet,
		Path:        "/api/v1/receipts/{receipt_id}",
		Summary:     "Get receipt detail",
		Description: "Returns one persisted webhook receipt together with the normalized events derived from it and any delivery rows created for those events. This is the quickest way to debug a webhook that was accepted but ended up unrouted, ignored, or partially delivered.",
		Tags:        []string{"Receipts"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.getReceipt)

	huma.Register(api, huma.Operation{
		OperationID: "receipts-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/receipts",
		Summary:     "List receipts",
		Description: "Returns persisted webhook receipts with optional filters for status, source, integration, and creation time window. Use this to locate unrouted hooks, duplicates, and ignored payloads before drilling into a specific receipt detail record.",
		Tags:        []string{"Receipts"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.listReceipts)

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
		OperationID: "integrations-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/integrations",
		Summary:     "List integrations",
		Description: "Returns the active dynamically managed webhook integrations with secrets redacted.",
		Tags:        []string{"Integrations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.listIntegrations)

	huma.Register(api, huma.Operation{
		OperationID: "integration-detail",
		Method:      http.MethodGet,
		Path:        "/api/v1/integrations/{integration_id}",
		Summary:     "Get integration detail",
		Description: "Returns one dynamically managed webhook integration with secret fields redacted.",
		Tags:        []string{"Integrations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.getIntegration)

	huma.Register(api, huma.Operation{
		OperationID: "integration-create",
		Method:      http.MethodPost,
		Path:        "/api/v1/integrations",
		Summary:     "Create integration",
		Description: "Creates a dynamically managed webhook integration, persists its encrypted secret fields, and reloads the live integration registry.",
		Tags:        []string{"Integrations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.createIntegration)

	huma.Register(api, huma.Operation{
		OperationID: "integration-update",
		Method:      http.MethodPut,
		Path:        "/api/v1/integrations/{integration_id}",
		Summary:     "Update integration",
		Description: "Updates one dynamically managed integration. Submitting `\"[REDACTED]\"` or omitting a secret field preserves the existing encrypted value.",
		Tags:        []string{"Integrations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.updateIntegration)

	huma.Register(api, huma.Operation{
		OperationID: "integration-delete",
		Method:      http.MethodDelete,
		Path:        "/api/v1/integrations/{integration_id}",
		Summary:     "Delete integration",
		Description: "Deletes one dynamically managed integration and reloads the live integration registry.",
		Tags:        []string{"Integrations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.deleteIntegration)

	huma.Register(api, huma.Operation{
		OperationID: "destinations-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/destinations",
		Summary:     "List destinations",
		Description: "Returns the active dynamically managed delivery destinations with sensitive fields redacted.",
		Tags:        []string{"Destinations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.listDestinations)

	huma.Register(api, huma.Operation{
		OperationID: "destination-detail",
		Method:      http.MethodGet,
		Path:        "/api/v1/destinations/{destination_id}",
		Summary:     "Get destination detail",
		Description: "Returns one dynamically managed delivery destination with secret fields redacted.",
		Tags:        []string{"Destinations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.getDestination)

	huma.Register(api, huma.Operation{
		OperationID: "destination-create",
		Method:      http.MethodPost,
		Path:        "/api/v1/destinations",
		Summary:     "Create destination",
		Description: "Creates a dynamically managed delivery destination, persists its encrypted secret fields, and reloads the live destination registry.",
		Tags:        []string{"Destinations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.createDestination)

	huma.Register(api, huma.Operation{
		OperationID: "destination-update",
		Method:      http.MethodPut,
		Path:        "/api/v1/destinations/{destination_id}",
		Summary:     "Update destination",
		Description: "Updates one dynamically managed destination. Submitting `\"[REDACTED]\"` or omitting a secret field preserves the existing encrypted value.",
		Tags:        []string{"Destinations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.updateDestination)

	huma.Register(api, huma.Operation{
		OperationID: "destination-delete",
		Method:      http.MethodDelete,
		Path:        "/api/v1/destinations/{destination_id}",
		Summary:     "Delete destination",
		Description: "Deletes one dynamically managed destination and reloads the live destination registry.",
		Tags:        []string{"Destinations"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.deleteDestination)

	huma.Register(api, huma.Operation{
		OperationID: "routes-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/routes",
		Summary:     "List routes",
		Description: "Returns the current persisted routing rules that the in-memory routing engine evaluates for normalized events. Each rule is additive, and duplicate destination matches are collapsed during delivery creation. Empty selector arrays mean 'match anything'; blank selector values are rejected at write time.",
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
		Description: "Creates a new persisted routing rule. The application validates referenced destinations and selector values, stores the rule durably, and reloads the in-memory routing engine after the write succeeds. Omit any selector field you want to behave as a wildcard.",
		Tags:        []string{"Routes"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, handler.createRoute)

	huma.Register(api, huma.Operation{
		OperationID: "route-update",
		Method:      http.MethodPut,
		Path:        "/api/v1/routes/{route_id}",
		Summary:     "Update route",
		Description: "Replaces the stored match criteria and destination targets for one routing rule, then refreshes the active in-memory routing engine so new events start using the updated policy immediately. Omit any selector field you want to behave as a wildcard; blank strings are rejected.",
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
