package app

import (
	"context"
	"errors"
	"sync"

	"github.com/fanboykun/webhook-hub/internal/clock"
	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/ingress"
	"github.com/fanboykun/webhook-hub/internal/routing"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"github.com/fanboykun/webhook-hub/internal/storage"
)

var (
	ErrUnauthorized             = errors.New("admin unauthorized")
	ErrRouteNotFound            = errors.New("route not found")
	ErrDeliveryNotFound         = errors.New("delivery not found")
	ErrReceiptNotFound          = errors.New("receipt not found")
	ErrIntegrationNotFound      = errors.New("integration not found")
	ErrDestinationNotFound      = errors.New("destination not found")
	ErrRendererProfileNotFound  = errors.New("renderer profile not found")
	ErrDestinationInUse         = errors.New("destination is in use")
	ErrRendererProfileInUse     = errors.New("renderer profile is in use")
	ErrDynamicConfigUnavailable = errors.New("dynamic config encryption is not configured")
	ErrRuntimeReloadRequired    = errors.New("runtime reload required")
)

type WebhookService interface {
	Handle(ctx context.Context, source domain.Source, req ingress.InboundRequest) (domain.IngestResult, error)
}

type Service struct {
	cfg          config.Config
	clock        clock.Clock
	store        storage.AdminStore
	webhook      WebhookService
	router       *routing.Engine
	integrations *runtimeconfig.IntegrationRegistry
	destinations *runtimeconfig.DestinationRegistry
	profiles     *runtimeconfig.RendererProfileRegistry
	dynamicMu    sync.Mutex
}

func NewService(cfg config.Config, clk clock.Clock, store storage.AdminStore, webhook WebhookService, router *routing.Engine, integrations *runtimeconfig.IntegrationRegistry, destinations *runtimeconfig.DestinationRegistry, profiles *runtimeconfig.RendererProfileRegistry) *Service {
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
