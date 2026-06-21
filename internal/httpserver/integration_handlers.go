package httpserver

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/fanboykun/webhook-hub/internal/app"
	"github.com/fanboykun/webhook-hub/internal/domain"
)

func (h *Handler) listIntegrations(ctx context.Context, input *listIntegrationsInput) (*integrationsResponse, error) {
	items, err := h.service.ListIntegrations(ctx, domain.Source(input.Source))
	if err != nil {
		if errors.Is(err, app.ErrDynamicConfigUnavailable) {
			return nil, huma.Error503ServiceUnavailable("integrations unavailable")
		}
		return nil, huma.Error503ServiceUnavailable("integrations unavailable")
	}
	out := &integrationsResponse{}
	out.Body.Items = make([]integrationModel, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, integrationModelFromDomain(item))
	}
	return out, nil
}

func (h *Handler) getIntegration(ctx context.Context, input *integrationDetailInput) (*integrationResponse, error) {
	item, err := h.service.GetIntegration(ctx, input.IntegrationID)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrDynamicConfigUnavailable):
			return nil, huma.Error503ServiceUnavailable("integration unavailable")
		case errors.Is(err, app.ErrIntegrationNotFound):
			return nil, huma.Error404NotFound("integration not found")
		default:
			return nil, huma.Error503ServiceUnavailable("integration unavailable")
		}
	}
	return &integrationResponse{Body: integrationModelFromDomain(item)}, nil
}

func (h *Handler) createIntegration(ctx context.Context, input *createIntegrationInput) (*integrationResponse, error) {
	item, err := h.service.CreateIntegration(ctx, domainIntegrationFromRequestModel(input.Body))
	if err != nil {
		switch {
		case errors.Is(err, app.ErrDynamicConfigUnavailable):
			return nil, huma.Error503ServiceUnavailable("integrations unavailable")
		case errors.Is(err, app.ErrRuntimeReloadRequired):
			return nil, huma.Error503ServiceUnavailable("integration persisted but runtime reload failed")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	return &integrationResponse{Body: integrationModelFromDomain(item)}, nil
}

func (h *Handler) updateIntegration(ctx context.Context, input *updateIntegrationInput) (*integrationResponse, error) {
	item := domainIntegrationFromRequestModel(input.Body)
	item.ID = input.IntegrationID
	updated, err := h.service.UpdateIntegration(ctx, item)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrDynamicConfigUnavailable):
			return nil, huma.Error503ServiceUnavailable("integration unavailable")
		case errors.Is(err, app.ErrIntegrationNotFound):
			return nil, huma.Error404NotFound("integration not found")
		case errors.Is(err, app.ErrRuntimeReloadRequired):
			return nil, huma.Error503ServiceUnavailable("integration persisted but runtime reload failed")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	return &integrationResponse{Body: integrationModelFromDomain(updated)}, nil
}

func (h *Handler) deleteIntegration(ctx context.Context, input *deleteIntegrationInput) (*deleteEntityOutput, error) {
	if err := h.service.DeleteIntegration(ctx, input.IntegrationID); err != nil {
		switch {
		case errors.Is(err, app.ErrDynamicConfigUnavailable):
			return nil, huma.Error503ServiceUnavailable("integration unavailable")
		case errors.Is(err, app.ErrIntegrationNotFound):
			return nil, huma.Error404NotFound("integration not found")
		case errors.Is(err, app.ErrRuntimeReloadRequired):
			return nil, huma.Error503ServiceUnavailable("integration deleted but runtime reload failed")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	out := &deleteEntityOutput{}
	out.Body.Deleted = true
	return out, nil
}
