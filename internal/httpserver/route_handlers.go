package httpserver

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/fanboykun/webhook-hub/internal/app"
)

func (h *Handler) listRoutes(ctx context.Context, input *listRoutesInput) (*routesResponse, error) {
	routes, err := h.service.ListRoutes(ctx)
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("routes unavailable")
	}
	out := &routesResponse{}
	out.Body.Routes = routeModelsFromDomain(routes)
	return out, nil
}

func (h *Handler) getRoute(ctx context.Context, input *routeDetailInput) (*routeResponse, error) {
	route, err := h.service.GetRoute(ctx, input.RouteID)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrRouteNotFound):
			return nil, huma.Error404NotFound("route not found")
		default:
			return nil, huma.Error503ServiceUnavailable("route unavailable")
		}
	}
	return &routeResponse{Body: routeModelFromDomain(route)}, nil
}

func (h *Handler) createRoute(ctx context.Context, input *createRouteInput) (*routeResponse, error) {
	route, err := h.service.CreateRoute(ctx, domainRouteFromModel(input.Body))
	if err != nil {
		switch {
		case errors.Is(err, app.ErrRuntimeReloadRequired):
			return nil, huma.Error503ServiceUnavailable("route persisted but runtime reload failed")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	return &routeResponse{Body: routeModelFromDomain(route)}, nil
}

func (h *Handler) updateRoute(ctx context.Context, input *updateRouteInput) (*routeResponse, error) {
	route := domainRouteFromModel(input.Body)
	route.ID = input.RouteID
	updated, err := h.service.UpdateRoute(ctx, route)
	if err != nil {
		switch {
		case errors.Is(err, app.ErrRouteNotFound):
			return nil, huma.Error404NotFound("route not found")
		case errors.Is(err, app.ErrRuntimeReloadRequired):
			return nil, huma.Error503ServiceUnavailable("route persisted but runtime reload failed")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	return &routeResponse{Body: routeModelFromDomain(updated)}, nil
}

func (h *Handler) deleteRoute(ctx context.Context, input *routeDeleteInput) (*routeDeleteOutput, error) {
	if err := h.service.DeleteRoute(ctx, input.RouteID); err != nil {
		switch {
		case errors.Is(err, app.ErrRouteNotFound):
			return nil, huma.Error404NotFound("route not found")
		case errors.Is(err, app.ErrRuntimeReloadRequired):
			return nil, huma.Error503ServiceUnavailable("route deleted but runtime reload failed")
		default:
			return nil, huma.Error400BadRequest(err.Error())
		}
	}
	out := &routeDeleteOutput{}
	out.Body.Deleted = true
	return out, nil
}
