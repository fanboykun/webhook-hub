package httpserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/fanboykun/webhook-hub/internal/app"
	"github.com/fanboykun/webhook-hub/internal/domain"
)

func (h *Handler) getDelivery(ctx context.Context, input *deliveryDetailInput) (*deliveryResponse, error) {
	delivery, err := h.service.GetDelivery(ctx, input.DeliveryID)
	if err != nil {
		switch {
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
	receipt, err := h.service.GetReceipt(ctx, input.ReceiptID)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrReceiptNotFound):
			return nil, huma.Error404NotFound("receipt not found")
		default:
			return nil, huma.Error503ServiceUnavailable("receipt unavailable")
		}
	}

	out := &receiptResponse{}
	out.Body.ID = receipt.Receipt.ID
	out.Body.Source = routeSource(receipt.Receipt.Source)
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

	page, err := h.service.ListReceipts(ctx, filter)
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("receipts unavailable")
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

	page, err := h.service.ListDeliveries(ctx, filter)
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("deliveries unavailable")
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
	if err := h.service.RetryDelivery(ctx, input.DeliveryID); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	out := &retryDeliveryOutput{Status: http.StatusAccepted}
	out.Body.DeliveryID = input.DeliveryID
	out.Body.Status = string(domain.DeliveryRetryWait)
	return out, nil
}
