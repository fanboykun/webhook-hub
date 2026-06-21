package app

import (
	"context"
	"errors"
	"time"

	"github.com/fanboykun/webhook-hub/internal/clock"
	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/ingress"
	"github.com/fanboykun/webhook-hub/internal/routing"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
)

var (
	ErrUnauthorized             = errors.New("admin unauthorized")
	ErrRouteNotFound            = errors.New("route not found")
	ErrDeliveryNotFound         = errors.New("delivery not found")
	ErrReceiptNotFound          = errors.New("receipt not found")
	ErrIntegrationNotFound      = errors.New("integration not found")
	ErrDestinationNotFound      = errors.New("destination not found")
	ErrRendererProfileNotFound  = errors.New("renderer profile not found")
	ErrDynamicConfigUnavailable = errors.New("dynamic config encryption is not configured")
	ErrRuntimeReloadRequired    = errors.New("runtime reload required")
)

type Store interface {
	ListIntegrations(ctx context.Context) ([]domain.ManagedIntegration, error)
	GetIntegration(ctx context.Context, id string) (domain.ManagedIntegration, error)
	CreateIntegration(ctx context.Context, integration domain.ManagedIntegration) error
	UpdateIntegration(ctx context.Context, integration domain.ManagedIntegration) error
	DeleteIntegration(ctx context.Context, id string) error
	ListDestinations(ctx context.Context) ([]domain.ManagedDestination, error)
	GetDestination(ctx context.Context, id string) (domain.ManagedDestination, error)
	CreateDestination(ctx context.Context, destination domain.ManagedDestination) error
	UpdateDestination(ctx context.Context, destination domain.ManagedDestination) error
	DeleteDestination(ctx context.Context, id string) error
	ListRendererProfiles(ctx context.Context) ([]domain.ManagedRendererProfile, error)
	GetRendererProfile(ctx context.Context, id string) (domain.ManagedRendererProfile, error)
	CreateRendererProfile(ctx context.Context, profile domain.ManagedRendererProfile) error
	UpdateRendererProfile(ctx context.Context, profile domain.ManagedRendererProfile) error
	DeleteRendererProfile(ctx context.Context, id string) error
	ListRoutes(ctx context.Context) ([]domain.Route, error)
	GetRoute(ctx context.Context, id string) (domain.Route, error)
	CreateRoute(ctx context.Context, route domain.Route) error
	UpdateRoute(ctx context.Context, route domain.Route) error
	DeleteRoute(ctx context.Context, id string) error
	GetReceipt(ctx context.Context, id string) (domain.Receipt, error)
	ListReceipts(ctx context.Context, filter domain.ReceiptFilter) (domain.ReceiptPage, error)
	GetDelivery(ctx context.Context, id string) (domain.Delivery, error)
	ListEventsByReceipt(ctx context.Context, receiptID string) ([]domain.Event, error)
	ListDeliveriesByEventIDs(ctx context.Context, eventIDs []string) (map[string][]domain.Delivery, error)
	ListDeliveries(ctx context.Context, filter domain.DeliveryFilter) (domain.DeliveryPage, error)
	RetryDelivery(ctx context.Context, id string, now time.Time) error
	Ping(ctx context.Context) error
}

type WebhookService interface {
	Handle(ctx context.Context, source domain.Source, req ingress.InboundRequest) (domain.IngestResult, error)
}

type Service struct {
	cfg          config.Config
	clock        clock.Clock
	store        Store
	webhook      WebhookService
	router       *routing.Engine
	integrations *runtimeconfig.IntegrationRegistry
	destinations *runtimeconfig.DestinationRegistry
	profiles     *runtimeconfig.RendererProfileRegistry
}

func NewService(cfg config.Config, clk clock.Clock, store Store, webhook WebhookService, router *routing.Engine, integrations *runtimeconfig.IntegrationRegistry, destinations *runtimeconfig.DestinationRegistry, profiles *runtimeconfig.RendererProfileRegistry) *Service {
	return &Service{
		cfg:          cfg,
		clock:        clk,
		store:        store,
		webhook:      webhook,
		router:       router,
		integrations: integrations,
		destinations: destinations,
		profiles:     profiles,
	}
}

func (s *Service) HandleWebhook(ctx context.Context, source domain.Source, req ingress.InboundRequest) (domain.IngestResult, error) {
	return s.webhook.Handle(ctx, source, req)
}

func (s *Service) CheckReady(ctx context.Context) error {
	return s.store.Ping(ctx)
}
