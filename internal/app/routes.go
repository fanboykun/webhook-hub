package app

import (
	"context"
	"errors"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"gorm.io/gorm"
)

func (s *Service) ListRoutes(ctx context.Context) ([]domain.Route, error) {
	return s.store.ListRoutes(ctx)
}

func (s *Service) GetRoute(ctx context.Context, routeID string) (domain.Route, error) {
	route, err := s.store.GetRoute(ctx, routeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Route{}, ErrRouteNotFound
		}
		return domain.Route{}, err
	}
	return route, nil
}

func (s *Service) CreateRoute(ctx context.Context, route domain.Route) (domain.Route, error) {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	route = normalizeRoute(route)
	now := s.clock.Now().UTC()
	route.CreatedAt = now
	route.UpdatedAt = now
	if err := s.validateRoute(route); err != nil {
		return domain.Route{}, err
	}
	if err := s.store.CreateRoute(ctx, route); err != nil {
		return domain.Route{}, err
	}
	if err := s.loadRoutesLocked(ctx); err != nil {
		return domain.Route{}, errors.Join(ErrRuntimeReloadRequired, err)
	}
	return route, nil
}

func (s *Service) UpdateRoute(ctx context.Context, route domain.Route) (domain.Route, error) {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	route = normalizeRoute(route)
	current, err := s.store.GetRoute(ctx, route.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Route{}, ErrRouteNotFound
		}
		return domain.Route{}, err
	}
	route.CreatedAt = current.CreatedAt
	route.UpdatedAt = s.clock.Now().UTC()
	if err := s.validateRoute(route); err != nil {
		return domain.Route{}, err
	}
	if err := s.store.UpdateRoute(ctx, route); err != nil {
		return domain.Route{}, err
	}
	if err := s.loadRoutesLocked(ctx); err != nil {
		return domain.Route{}, errors.Join(ErrRuntimeReloadRequired, err)
	}
	return route, nil
}

func (s *Service) DeleteRoute(ctx context.Context, routeID string) error {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	if _, err := s.store.GetRoute(ctx, routeID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRouteNotFound
		}
		return err
	}
	if err := s.store.DeleteRoute(ctx, routeID); err != nil {
		return err
	}
	if err := s.loadRoutesLocked(ctx); err != nil {
		return errors.Join(ErrRuntimeReloadRequired, err)
	}
	return nil
}
