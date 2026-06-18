package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/iweka-dev/webhook-hub/internal/clock"
	"github.com/iweka-dev/webhook-hub/internal/config"
	"github.com/iweka-dev/webhook-hub/internal/domain"
	"github.com/iweka-dev/webhook-hub/internal/ingress"
	"github.com/iweka-dev/webhook-hub/internal/routing"
	"gorm.io/gorm"
)

var (
	ErrUnauthorized     = errors.New("admin unauthorized")
	ErrRouteNotFound    = errors.New("route not found")
	ErrDeliveryNotFound = errors.New("delivery not found")
)

type Store interface {
	ListRoutes(ctx context.Context) ([]domain.Route, error)
	GetRoute(ctx context.Context, id string) (domain.Route, error)
	CreateRoute(ctx context.Context, route domain.Route) error
	UpdateRoute(ctx context.Context, route domain.Route) error
	DeleteRoute(ctx context.Context, id string) error
	GetDelivery(ctx context.Context, id string) (domain.Delivery, error)
	ListDeliveries(ctx context.Context, filter domain.DeliveryFilter) (domain.DeliveryPage, error)
	RetryDelivery(ctx context.Context, id string, now time.Time) error
	Ping(ctx context.Context) error
}

type WebhookService interface {
	Handle(ctx context.Context, source domain.Source, req ingress.InboundRequest) (domain.IngestResult, error)
}

type Service struct {
	cfg     config.Config
	clock   clock.Clock
	store   Store
	webhook WebhookService
	router  *routing.Engine
}

func NewService(cfg config.Config, clk clock.Clock, store Store, webhook WebhookService, router *routing.Engine) *Service {
	return &Service{cfg: cfg, clock: clk, store: store, webhook: webhook, router: router}
}

func (s *Service) LoadRoutes(ctx context.Context) error {
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return err
	}
	s.router.Replace(routes)
	return nil
}

func (s *Service) HandleWebhook(ctx context.Context, source domain.Source, req ingress.InboundRequest) (domain.IngestResult, error) {
	return s.webhook.Handle(ctx, source, req)
}

func (s *Service) CheckReady(ctx context.Context) error {
	return s.store.Ping(ctx)
}

func (s *Service) GetDelivery(ctx context.Context, authorization, deliveryID string) (domain.Delivery, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.Delivery{}, err
	}
	delivery, err := s.store.GetDelivery(ctx, deliveryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Delivery{}, ErrDeliveryNotFound
		}
		return domain.Delivery{}, err
	}
	return delivery, nil
}

func (s *Service) ListDeliveries(ctx context.Context, authorization string, filter domain.DeliveryFilter) (domain.DeliveryPage, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.DeliveryPage{}, err
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	return s.store.ListDeliveries(ctx, filter)
}

func (s *Service) RetryDelivery(ctx context.Context, authorization, deliveryID string) error {
	if err := s.authorize(authorization); err != nil {
		return err
	}
	return s.store.RetryDelivery(ctx, deliveryID, s.clock.Now().UTC())
}

func (s *Service) ListRoutes(ctx context.Context, authorization string) ([]domain.Route, error) {
	if err := s.authorize(authorization); err != nil {
		return nil, err
	}
	return s.store.ListRoutes(ctx)
}

func (s *Service) GetRoute(ctx context.Context, authorization, routeID string) (domain.Route, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.Route{}, err
	}
	route, err := s.store.GetRoute(ctx, routeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Route{}, ErrRouteNotFound
		}
		return domain.Route{}, err
	}
	return route, nil
}

func (s *Service) CreateRoute(ctx context.Context, authorization string, route domain.Route) (domain.Route, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.Route{}, err
	}
	now := s.clock.Now().UTC()
	route.CreatedAt = now
	route.UpdatedAt = now
	if err := s.validateRoute(route); err != nil {
		return domain.Route{}, err
	}
	if err := s.store.CreateRoute(ctx, route); err != nil {
		return domain.Route{}, err
	}
	if err := s.LoadRoutes(ctx); err != nil {
		return domain.Route{}, err
	}
	return route, nil
}

func (s *Service) UpdateRoute(ctx context.Context, authorization string, route domain.Route) (domain.Route, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.Route{}, err
	}
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
	if err := s.LoadRoutes(ctx); err != nil {
		return domain.Route{}, err
	}
	return route, nil
}

func (s *Service) DeleteRoute(ctx context.Context, authorization, routeID string) error {
	if err := s.authorize(authorization); err != nil {
		return err
	}
	if _, err := s.store.GetRoute(ctx, routeID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRouteNotFound
		}
		return err
	}
	if err := s.store.DeleteRoute(ctx, routeID); err != nil {
		return err
	}
	return s.LoadRoutes(ctx)
}

func (s *Service) validateRoute(route domain.Route) error {
	if route.ID == "" {
		return errors.New("route id is required")
	}
	if len(route.Destinations) == 0 {
		return errors.New("route destinations must not be empty")
	}
	for _, destinationID := range route.Destinations {
		if _, exists := s.cfg.Destinations[destinationID]; !exists {
			return errors.New("route references unknown destination " + destinationID)
		}
	}
	return nil
}

func (s *Service) authorize(authorization string) error {
	if s.cfg.API.ResolvedAdminToken == "" {
		return ErrUnauthorized
	}
	expected := "Bearer " + s.cfg.API.ResolvedAdminToken
	if subtle.ConstantTimeCompare([]byte(authorization), []byte(expected)) != 1 {
		return ErrUnauthorized
	}
	return nil
}
