package httpserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/iweka-dev/webhook-hub/internal/app"
	"github.com/iweka-dev/webhook-hub/internal/domain"
	"github.com/iweka-dev/webhook-hub/internal/ingress"
)

type Handler struct {
	service *app.Service
}

func NewHandler(service *app.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) handleWatcherWebhook(ctx context.Context, input *watcherWebhookInput) (*watcherWebhookOutput, error) {
	headers := http.Header{}
	headers.Set("webhook-id", input.WebhookID)
	headers.Set("webhook-timestamp", input.Timestamp)
	headers.Set("webhook-signature", input.Signature)
	headers.Set("X-Watcher-Event", input.Event)
	headers.Set("X-Watcher-Event-ID", input.WebhookID)
	headers.Set("X-Watcher-Delivery-ID", input.DeliveryID)
	headers.Set("Content-Type", "application/json")

	return h.handleWebhook(ctx, domain.SourceWatcher, ingress.InboundRequest{
		IntegrationID: input.IntegrationID,
		Headers:       headers,
		RawBody:       input.RawBody,
		ReceivedAt:    time.Now().UTC(),
	})
}

func (h *Handler) handleGitHubWebhook(ctx context.Context, input *githubWebhookInput) (*watcherWebhookOutput, error) {
	return h.handleWebhook(ctx, domain.SourceGitHub, ingress.InboundRequest{
		IntegrationID: input.IntegrationID,
		Headers: http.Header{
			"X-GitHub-Event":      []string{input.Event},
			"X-GitHub-Delivery":   []string{input.DeliveryID},
			"X-Hub-Signature-256": []string{input.Signature},
			"Content-Type":        []string{"application/json"},
		},
		RawBody:    input.RawBody,
		ReceivedAt: time.Now().UTC(),
	})
}

func (h *Handler) handleWebhook(ctx context.Context, source domain.Source, req ingress.InboundRequest) (*watcherWebhookOutput, error) {
	result, err := h.service.HandleWebhook(ctx, source, req)
	if err != nil {
		switch err {
		case ingress.ErrUnknownIntegration:
			return nil, huma.Error404NotFound("unknown integration")
		case ingress.ErrWrongSource, ingress.ErrMalformedPayload:
			return nil, huma.Error400BadRequest(err.Error())
		case ingress.ErrUnauthorized:
			return nil, huma.Error401Unauthorized("invalid signature")
		default:
			return nil, huma.Error503ServiceUnavailable("ingest failed")
		}
	}

	out := &watcherWebhookOutput{Status: http.StatusAccepted}
	out.Body.ReceiptID = result.ReceiptID
	out.Body.Status = string(result.Status)
	out.Body.Duplicate = result.Duplicate
	out.Body.EventCount = result.EventCount
	out.Body.DeliveryCount = result.DeliveryCount
	return out, nil
}

func (h *Handler) healthLive(ctx context.Context, input *struct{}) (*healthResponse, error) {
	return &healthResponse{Body: map[string]string{"status": "ok"}}, nil
}

func (h *Handler) healthReady(ctx context.Context, input *struct{}) (*healthResponse, error) {
	if err := h.service.CheckReady(ctx); err != nil {
		return nil, huma.Error503ServiceUnavailable("database unavailable")
	}
	return &healthResponse{Body: map[string]string{"status": "ready"}}, nil
}

func (h *Handler) getDelivery(ctx context.Context, input *deliveryDetailInput) (*deliveryResponse, error) {
	delivery, err := h.service.GetDelivery(ctx, input.Authorization, input.DeliveryID)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			return nil, huma.Error401Unauthorized("unauthorized")
		case errors.Is(err, app.ErrDeliveryNotFound):
			return nil, huma.Error404NotFound("delivery not found")
		default:
			return nil, huma.Error503ServiceUnavailable("delivery unavailable")
		}
	}

	out := &deliveryResponse{}
	out.Body.ID = delivery.ID
	out.Body.EventID = delivery.EventID
	out.Body.DestinationID = delivery.DestinationID
	out.Body.DestinationType = string(delivery.DestinationType)
	out.Body.Status = string(delivery.Status)
	out.Body.AttemptCount = delivery.AttemptCount
	out.Body.MaxAttempts = delivery.MaxAttempts
	out.Body.NextAttemptAt = delivery.NextAttemptAt
	out.Body.LockedBy = delivery.LockedBy
	out.Body.LockedUntil = delivery.LockedUntil
	out.Body.ProviderMessageID = delivery.ProviderMessageID
	out.Body.LastErrorCode = delivery.LastErrorCode
	out.Body.LastError = delivery.LastError
	out.Body.SentAt = delivery.SentAt
	out.Body.CreatedAt = delivery.CreatedAt
	out.Body.UpdatedAt = delivery.UpdatedAt
	return out, nil
}

func (h *Handler) getReceipt(ctx context.Context, input *receiptDetailInput) (*receiptResponse, error) {
	receipt, err := h.service.GetReceipt(ctx, input.Authorization, input.ReceiptID)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			return nil, huma.Error401Unauthorized("unauthorized")
		case errors.Is(err, app.ErrReceiptNotFound):
			return nil, huma.Error404NotFound("receipt not found")
		default:
			return nil, huma.Error503ServiceUnavailable("receipt unavailable")
		}
	}

	out := &receiptResponse{}
	out.Body.ID = receipt.Receipt.ID
	out.Body.Source = string(receipt.Receipt.Source)
	out.Body.IntegrationID = receipt.Receipt.IntegrationID
	out.Body.SourceDeliveryID = receipt.Receipt.SourceDeliveryID
	out.Body.SourceEventType = receipt.Receipt.SourceEventType
	out.Body.Status = string(receipt.Receipt.Status)
	out.Body.IgnoreReason = receipt.Receipt.IgnoreReason
	out.Body.ReceivedAt = receipt.Receipt.ReceivedAt
	out.Body.CreatedAt = receipt.Receipt.CreatedAt
	out.Body.Events = make([]receiptEvent, 0, len(receipt.Events))
	for _, event := range receipt.Events {
		out.Body.Events = append(out.Body.Events, receiptEventFromDomain(event, receipt.Deliveries[event.ID]))
	}
	return out, nil
}

func (h *Handler) listReceipts(ctx context.Context, input *listReceiptsInput) (*receiptsResponse, error) {
	filter := domain.ReceiptFilter{
		Status:        domain.ReceiptStatus(input.Status),
		Source:        domain.Source(input.Source),
		IntegrationID: input.IntegrationID,
		Limit:         input.Limit,
		Cursor:        input.Cursor,
	}

	if input.From != "" {
		from, err := time.Parse(time.RFC3339, input.From)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid from timestamp, expected RFC3339")
		}
		filter.From = &from
	}
	if input.To != "" {
		to, err := time.Parse(time.RFC3339, input.To)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid to timestamp, expected RFC3339")
		}
		filter.To = &to
	}

	page, err := h.service.ListReceipts(ctx, input.Authorization, filter)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			return nil, huma.Error401Unauthorized("unauthorized")
		default:
			return nil, huma.Error503ServiceUnavailable("receipts unavailable")
		}
	}

	out := &receiptsResponse{}
	out.Body.Items = make([]receiptItem, 0, len(page.Items))
	for _, receipt := range page.Items {
		out.Body.Items = append(out.Body.Items, receiptItemFromDomain(receipt))
	}
	out.Body.NextCursor = page.NextCursor
	return out, nil
}

func (h *Handler) listDeliveries(ctx context.Context, input *listDeliveriesInput) (*deliveriesResponse, error) {
	filter := domain.DeliveryFilter{
		Status:        domain.DeliveryStatus(input.Status),
		DestinationID: input.DestinationID,
		EventID:       input.EventID,
		Limit:         input.Limit,
		Cursor:        input.Cursor,
	}

	if input.From != "" {
		from, err := time.Parse(time.RFC3339, input.From)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid from timestamp, expected RFC3339")
		}
		filter.From = &from
	}
	if input.To != "" {
		to, err := time.Parse(time.RFC3339, input.To)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid to timestamp, expected RFC3339")
		}
		filter.To = &to
	}

	page, err := h.service.ListDeliveries(ctx, input.Authorization, filter)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			return nil, huma.Error401Unauthorized("unauthorized")
		default:
			return nil, huma.Error503ServiceUnavailable("deliveries unavailable")
		}
	}

	out := &deliveriesResponse{}
	out.Body.Items = make([]deliveryItem, 0, len(page.Items))
	for _, delivery := range page.Items {
		out.Body.Items = append(out.Body.Items, deliveryItemFromDomain(delivery))
	}
	out.Body.NextCursor = page.NextCursor
	return out, nil
}

func (h *Handler) retryDelivery(ctx context.Context, input *retryDeliveryInput) (*retryDeliveryOutput, error) {
	if err := h.service.RetryDelivery(ctx, input.Authorization, input.DeliveryID); err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			return nil, huma.Error401Unauthorized("unauthorized")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}

	out := &retryDeliveryOutput{Status: http.StatusAccepted}
	out.Body.DeliveryID = input.DeliveryID
	out.Body.Status = string(domain.DeliveryRetryWait)
	return out, nil
}

func (h *Handler) listRoutes(ctx context.Context, input *listRoutesInput) (*routesResponse, error) {
	routes, err := h.service.ListRoutes(ctx, input.Authorization)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			return nil, huma.Error401Unauthorized("unauthorized")
		default:
			return nil, huma.Error503ServiceUnavailable("routes unavailable")
		}
	}
	out := &routesResponse{}
	out.Body.Routes = routeModelsFromDomain(routes)
	return out, nil
}

func (h *Handler) getRoute(ctx context.Context, input *routeDetailInput) (*routeResponse, error) {
	route, err := h.service.GetRoute(ctx, input.Authorization, input.RouteID)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			return nil, huma.Error401Unauthorized("unauthorized")
		case errors.Is(err, app.ErrRouteNotFound):
			return nil, huma.Error404NotFound("route not found")
		default:
			return nil, huma.Error503ServiceUnavailable("route unavailable")
		}
	}
	return &routeResponse{Body: routeModelFromDomain(route)}, nil
}

func (h *Handler) createRoute(ctx context.Context, input *createRouteInput) (*routeResponse, error) {
	route, err := h.service.CreateRoute(ctx, input.Authorization, domainRouteFromModel(input.Body))
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			return nil, huma.Error401Unauthorized("unauthorized")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	return &routeResponse{Body: routeModelFromDomain(route)}, nil
}

func (h *Handler) updateRoute(ctx context.Context, input *updateRouteInput) (*routeResponse, error) {
	route := domainRouteFromModel(input.Body)
	route.ID = input.RouteID
	updated, err := h.service.UpdateRoute(ctx, input.Authorization, route)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			return nil, huma.Error401Unauthorized("unauthorized")
		case errors.Is(err, app.ErrRouteNotFound):
			return nil, huma.Error404NotFound("route not found")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	return &routeResponse{Body: routeModelFromDomain(updated)}, nil
}

func (h *Handler) deleteRoute(ctx context.Context, input *routeDeleteInput) (*routeDeleteOutput, error) {
	if err := h.service.DeleteRoute(ctx, input.Authorization, input.RouteID); err != nil {
		switch {
		case errors.Is(err, app.ErrUnauthorized):
			return nil, huma.Error401Unauthorized("unauthorized")
		case errors.Is(err, app.ErrRouteNotFound):
			return nil, huma.Error404NotFound("route not found")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	out := &routeDeleteOutput{}
	out.Body.Deleted = true
	return out, nil
}
