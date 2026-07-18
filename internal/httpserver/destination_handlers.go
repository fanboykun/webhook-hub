package httpserver

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/fanboykun/webhook-hub/internal/app"
	"github.com/fanboykun/webhook-hub/internal/domain"
)

func (h *Handler) listDestinations(ctx context.Context, input *listDestinationsInput) (*destinationsResponse, error) {
	items, err := h.service.ListDestinations(ctx, domain.DestinationType(input.Type))
	if err != nil {
		if errors.Is(err, app.ErrDynamicConfigUnavailable) {
			return nil, huma.Error503ServiceUnavailable("destinations unavailable")
		}
		return nil, huma.Error503ServiceUnavailable("destinations unavailable")
	}
	out := &destinationsResponse{}
	out.Body.Items = make([]destinationConfigModel, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, destinationModelFromDomain(item))
	}
	return out, nil
}

func (h *Handler) getDestination(ctx context.Context, input *destinationDetailInput) (*destinationResponse, error) {
	item, err := h.service.GetDestination(ctx, input.DestinationID)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrDynamicConfigUnavailable):
			return nil, huma.Error503ServiceUnavailable("destination unavailable")
		case errors.Is(err, app.ErrDestinationNotFound):
			return nil, huma.Error404NotFound("destination not found")
		default:
			return nil, huma.Error503ServiceUnavailable("destination unavailable")
		}
	}
	return &destinationResponse{Body: destinationModelFromDomain(item)}, nil
}

func (h *Handler) createDestination(ctx context.Context, input *createDestinationInput) (*destinationResponse, error) {
	item, err := h.service.CreateDestination(ctx, domainDestinationFromRequestModel(input.Body))
	if err != nil {
		switch {
		case errors.Is(err, app.ErrDynamicConfigUnavailable):
			return nil, huma.Error503ServiceUnavailable("destinations unavailable")
		case errors.Is(err, app.ErrRuntimeReloadRequired):
			return nil, huma.Error503ServiceUnavailable("destination persisted but runtime reload failed")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	return &destinationResponse{Body: destinationModelFromDomain(item)}, nil
}

func (h *Handler) updateDestination(ctx context.Context, input *updateDestinationInput) (*destinationResponse, error) {
	item := domainDestinationFromRequestModel(input.Body)
	item.ID = input.DestinationID
	updated, err := h.service.UpdateDestination(ctx, item)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrDynamicConfigUnavailable):
			return nil, huma.Error503ServiceUnavailable("destination unavailable")
		case errors.Is(err, app.ErrDestinationNotFound):
			return nil, huma.Error404NotFound("destination not found")
		case errors.Is(err, app.ErrDestinationInUse):
			return nil, huma.Error409Conflict(err.Error())
		case errors.Is(err, app.ErrRuntimeReloadRequired):
			return nil, huma.Error503ServiceUnavailable("destination persisted but runtime reload failed")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	return &destinationResponse{Body: destinationModelFromDomain(updated)}, nil
}

func (h *Handler) deleteDestination(ctx context.Context, input *deleteDestinationInput) (*deleteEntityOutput, error) {
	if err := h.service.DeleteDestination(ctx, input.DestinationID); err != nil {
		switch {
		case errors.Is(err, app.ErrDynamicConfigUnavailable):
			return nil, huma.Error503ServiceUnavailable("destination unavailable")
		case errors.Is(err, app.ErrDestinationNotFound):
			return nil, huma.Error404NotFound("destination not found")
		case errors.Is(err, app.ErrDestinationInUse):
			return nil, huma.Error409Conflict(err.Error())
		case errors.Is(err, app.ErrRuntimeReloadRequired):
			return nil, huma.Error503ServiceUnavailable("destination deleted but runtime reload failed")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	out := &deleteEntityOutput{}
	out.Body.Deleted = true
	return out, nil
}
