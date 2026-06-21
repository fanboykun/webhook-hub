package httpserver

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/fanboykun/webhook-hub/internal/app"
)

func (h *Handler) listRendererProfiles(ctx context.Context, input *listRendererProfilesInput) (*rendererProfilesResponse, error) {
	items, err := h.service.ListRendererProfiles(ctx)
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("renderer profiles unavailable")
	}
	out := &rendererProfilesResponse{}
	out.Body.Items = make([]rendererProfileModel, 0, len(items))
	for _, item := range items {
		out.Body.Items = append(out.Body.Items, rendererProfileModelFromDomain(item))
	}
	return out, nil
}

func (h *Handler) getRendererProfile(ctx context.Context, input *rendererProfileDetailInput) (*rendererProfileResponse, error) {
	item, err := h.service.GetRendererProfile(ctx, input.ProfileID)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrRendererProfileNotFound):
			return nil, huma.Error404NotFound("renderer profile not found")
		default:
			return nil, huma.Error503ServiceUnavailable("renderer profile unavailable")
		}
	}
	return &rendererProfileResponse{Body: rendererProfileModelFromDomain(item)}, nil
}

func (h *Handler) createRendererProfile(ctx context.Context, input *createRendererProfileInput) (*rendererProfileResponse, error) {
	item, err := domainRendererProfileFromRequestModel(input.Body)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	created, err := h.service.CreateRendererProfile(ctx, item)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrRuntimeReloadRequired):
			return nil, huma.Error503ServiceUnavailable("renderer profile persisted but runtime reload failed")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	return &rendererProfileResponse{Body: rendererProfileModelFromDomain(created)}, nil
}

func (h *Handler) updateRendererProfile(ctx context.Context, input *updateRendererProfileInput) (*rendererProfileResponse, error) {
	item, err := domainRendererProfileFromRequestModel(input.Body)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	item.ID = input.ProfileID
	updated, err := h.service.UpdateRendererProfile(ctx, item)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrRendererProfileNotFound):
			return nil, huma.Error404NotFound("renderer profile not found")
		case errors.Is(err, app.ErrRuntimeReloadRequired):
			return nil, huma.Error503ServiceUnavailable("renderer profile persisted but runtime reload failed")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	return &rendererProfileResponse{Body: rendererProfileModelFromDomain(updated)}, nil
}

func (h *Handler) deleteRendererProfile(ctx context.Context, input *deleteRendererProfileInput) (*deleteEntityOutput, error) {
	if err := h.service.DeleteRendererProfile(ctx, input.ProfileID); err != nil {
		switch {
		case errors.Is(err, app.ErrRendererProfileNotFound):
			return nil, huma.Error404NotFound("renderer profile not found")
		case errors.Is(err, app.ErrRuntimeReloadRequired):
			return nil, huma.Error503ServiceUnavailable("renderer profile deleted but runtime reload failed")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	out := &deleteEntityOutput{}
	out.Body.Deleted = true
	return out, nil
}
