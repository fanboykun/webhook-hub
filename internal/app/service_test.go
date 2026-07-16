package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/clock"
	"github.com/fanboykun/webhook-hub/internal/config"
	configcrypto "github.com/fanboykun/webhook-hub/internal/config/crypto"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/observability"
	"github.com/fanboykun/webhook-hub/internal/routing"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"github.com/fanboykun/webhook-hub/internal/storage/sqlite"
)

func TestCreateRouteNormalizesBlankSelectorValues(t *testing.T) {
	svc := newTestService()

	route, err := svc.CreateRoute(context.Background(), domain.Route{
		ID:          "watcher-all-events",
		Description: "Send all Watcher events to Slack",
		Match: domain.RouteMatchCriteria{
			Environments: []string{""},
			Severities:   []domain.Severity{""},
		},
		Destinations: []string{"slack-deployments"},
	})
	if err != nil {
		t.Fatalf("expected blank selectors to be normalized away, got %v", err)
	}
	if len(route.Match.Environments) != 0 {
		t.Fatalf("expected environments to be normalized away, got %+v", route.Match.Environments)
	}
	if len(route.Match.Severities) != 0 {
		t.Fatalf("expected severities to be normalized away, got %+v", route.Match.Severities)
	}
}

func TestCreateRouteRejectsUnknownEnumValues(t *testing.T) {
	svc := newTestService()

	_, err := svc.CreateRoute(context.Background(), domain.Route{
		ID: "watcher-all-events",
		Match: domain.RouteMatchCriteria{
			Sources: []domain.Source{domain.SourceWatcher},
			Types:   []string{"watcher.deployment.not-real"},
		},
		Destinations: []string{"slack-deployments"},
	})
	if err == nil || !strings.Contains(err.Error(), "route types contains unknown value") {
		t.Fatalf("expected unknown event type error, got %v", err)
	}
}

func TestCreateRouteAllowsWildcardSelectors(t *testing.T) {
	svc := newTestService()

	route, err := svc.CreateRoute(context.Background(), domain.Route{
		ID:          "watcher-all-events",
		Description: "Send all Watcher events to Slack",
		Match: domain.RouteMatchCriteria{
			Sources: []domain.Source{domain.SourceWatcher},
		},
		Destinations: []string{"slack-deployments"},
	})
	if err != nil {
		t.Fatalf("create route: %v", err)
	}
	if route.ID != "watcher-all-events" {
		t.Fatalf("unexpected route returned: %+v", route)
	}
}

func TestCreateRouteRejectsDestinationMissingFromDynamicRegistry(t *testing.T) {
	svc := newTestService()
	svc.destinations.Replace(nil)

	_, err := svc.CreateRoute(context.Background(), domain.Route{
		ID:           "watcher-all-events",
		Destinations: []string{"slack-deployments"},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown destination") {
		t.Fatalf("expected unknown destination error, got %v", err)
	}
}

func TestDeleteDestinationRejectsActiveDeliveries(t *testing.T) {
	store := &activeDeliveryStoreStub{seedStoreStub: seedStoreStub{
		destinations: []domain.ManagedDestination{{ID: "slack-deployments", Type: domain.DestinationSlack, WebhookURL: "https://example.invalid"}},
	}}
	svc := NewService(config.Config{}, clock.Real{}, store, nil, routing.New(nil), runtimeconfig.NewIntegrationRegistry(nil), runtimeconfig.NewDestinationRegistry(map[string]config.DestinationConfig{
		"slack-deployments": {Type: domain.DestinationSlack, ResolvedURL: "https://example.invalid"},
	}), runtimeconfig.NewRendererProfileRegistry(nil))

	err := svc.DeleteDestination(context.Background(), "slack-deployments")
	if !errors.Is(err, ErrDestinationInUse) {
		t.Fatalf("expected destination in use error, got %v", err)
	}
}

func TestBootstrapDynamicConfigSeedsAndReloadsRegistries(t *testing.T) {
	store := &seedStoreStub{}
	integrations := runtimeconfig.NewIntegrationRegistry(nil)
	destinations := runtimeconfig.NewDestinationRegistry(nil)
	profiles := runtimeconfig.NewRendererProfileRegistry(nil)
	router := routing.New(nil)
	svc := NewService(config.Config{
		Integrations: map[string]config.IntegrationConfig{
			"github-main": {
				Source:         domain.SourceGitHub,
				ResolvedSecret: "github-secret",
			},
		},
		Destinations: map[string]config.DestinationConfig{
			"slack-deployments": {
				Type:        domain.DestinationSlack,
				ResolvedURL: "https://example.invalid",
			},
		},
		RendererProfiles: map[string]config.ProfileConfig{
			"watcher-deployment-failed-detailed": {
				Source: domain.SourceWatcher,
				Key:    "watcher.deployment.failed",
				Templates: config.DestinationTemplates{
					Slack: &config.SlackTemplateConfig{
						Title: "{{.Title}}",
						Body:  "{{.Summary}}",
					},
				},
			},
		},
		Routes: []config.RouteConfig{{
			ID:           "watcher-failed",
			Match:        config.RouteMatchConfig{Types: []string{"watcher.deployment.failed"}},
			Destinations: []string{"slack-deployments"},
		}},
	}, clock.Real{}, store, nil, router, integrations, destinations, profiles)

	if err := svc.BootstrapDynamicConfig(context.Background()); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	if _, ok := integrations.Get("github-main"); !ok {
		t.Fatal("expected integration registry to be seeded")
	}
	if _, ok := destinations.Get("slack-deployments"); !ok {
		t.Fatal("expected destination registry to be seeded")
	}
	if _, ok := profiles.Get("watcher-deployment-failed-detailed"); !ok {
		t.Fatal("expected renderer profile registry to be seeded")
	}
	if routes := router.Snapshot(); len(routes) != 1 || routes[0].ID != "watcher-failed" {
		t.Fatalf("expected route engine to be seeded, got %+v", routes)
	}
}

func TestBootstrapDynamicConfigSeedsFreshSQLiteAndPreservesExistingState(t *testing.T) {
	cipher, err := configcrypto.NewFromString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatalf("create config cipher: %v", err)
	}
	cfg := config.Config{
		Logging: config.LoggingConfig{Format: "text"},
		Database: config.DatabaseConfig{
			Path:               t.TempDir() + "/gateway.db",
			BusyTimeout:        time.Second,
			MaxOpenConnections: 1,
		},
		Integrations: map[string]config.IntegrationConfig{
			"github-main": {Source: domain.SourceGitHub, ResolvedSecret: "github-secret"},
		},
		Destinations: map[string]config.DestinationConfig{
			"slack-deployments": {
				Type:             domain.DestinationSlack,
				ResolvedURL:      "https://example.invalid/slack",
				RendererProfiles: []string{"watcher-failed"},
			},
		},
		RendererProfiles: map[string]config.ProfileConfig{
			"watcher-failed": {
				Source: domain.SourceWatcher,
				Key:    "watcher.deployment.failed",
				Templates: config.DestinationTemplates{
					Slack: &config.SlackTemplateConfig{Title: "{{.Title}}", Body: "{{.Summary}}"},
				},
			},
		},
		Routes: []config.RouteConfig{{
			ID:           "watcher-failed",
			Description:  "default description",
			Match:        config.RouteMatchConfig{Types: []string{"watcher.deployment.failed"}},
			Destinations: []string{"slack-deployments"},
		}},
	}
	store, err := sqlite.OpenWithCipher(cfg.Database, observability.NewLogger(cfg.Logging), cipher)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	integrations := runtimeconfig.NewIntegrationRegistry(nil)
	destinations := runtimeconfig.NewDestinationRegistry(nil)
	profiles := runtimeconfig.NewRendererProfileRegistry(nil)
	router := routing.New(nil)
	svc := NewService(cfg, clock.Real{}, store, nil, router, integrations, destinations, profiles)
	if err := svc.BootstrapDynamicConfig(context.Background()); err != nil {
		t.Fatalf("bootstrap fresh sqlite: %v", err)
	}

	storedIntegrations, err := store.ListIntegrations(context.Background())
	if err != nil {
		t.Fatalf("list seeded integrations: %v", err)
	}
	storedDestinations, err := store.ListDestinations(context.Background())
	if err != nil {
		t.Fatalf("list seeded destinations: %v", err)
	}
	storedProfiles, err := store.ListRendererProfiles(context.Background())
	if err != nil {
		t.Fatalf("list seeded renderer profiles: %v", err)
	}
	storedRoutes, err := store.ListRoutes(context.Background())
	if err != nil {
		t.Fatalf("list seeded routes: %v", err)
	}
	if len(storedIntegrations) != 1 || len(storedDestinations) != 1 || len(storedProfiles) != 1 || len(storedRoutes) != 1 {
		t.Fatalf("expected all defaults to be seeded, got integrations=%d destinations=%d profiles=%d routes=%d", len(storedIntegrations), len(storedDestinations), len(storedProfiles), len(storedRoutes))
	}
	if _, ok := integrations.Get("github-main"); !ok {
		t.Fatal("expected seeded integration in live registry")
	}
	if _, ok := destinations.Get("slack-deployments"); !ok {
		t.Fatal("expected seeded destination in live registry")
	}
	if _, ok := profiles.Get("watcher-failed"); !ok {
		t.Fatal("expected seeded profile in live registry")
	}
	if got := router.Snapshot(); len(got) != 1 || got[0].ID != "watcher-failed" {
		t.Fatalf("expected seeded route in live engine, got %+v", got)
	}

	storedRoutes[0].Description = "operator-managed description"
	storedRoutes[0].UpdatedAt = time.Now().UTC()
	if err := store.UpdateRoute(context.Background(), storedRoutes[0]); err != nil {
		t.Fatalf("update persisted route: %v", err)
	}
	if err := svc.BootstrapDynamicConfig(context.Background()); err != nil {
		t.Fatalf("bootstrap existing sqlite: %v", err)
	}
	if got := router.Snapshot(); len(got) != 1 || got[0].Description != "operator-managed description" {
		t.Fatalf("expected existing route state to be preserved, got %+v", got)
	}
}

func TestBootstrapDynamicConfigRejectsInvalidPersistedRouteBeforePublication(t *testing.T) {
	store := &seedStoreStub{
		destinations: []domain.ManagedDestination{{
			ID:         "slack-deployments",
			Type:       domain.DestinationSlack,
			WebhookURL: "https://example.invalid/slack",
		}},
		routes: []domain.Route{{
			ID:           "broken-route",
			Destinations: []string{"missing-destination"},
		}},
	}
	existing := domain.Route{ID: "existing-route", Destinations: []string{"slack-deployments"}}
	router := routing.New([]domain.Route{existing})
	svc := NewService(
		config.Config{},
		clock.Real{},
		store,
		nil,
		router,
		runtimeconfig.NewIntegrationRegistry(nil),
		runtimeconfig.NewDestinationRegistry(nil),
		runtimeconfig.NewRendererProfileRegistry(nil),
	)

	err := svc.BootstrapDynamicConfig(context.Background())
	if err == nil || !strings.Contains(err.Error(), "persisted route \"broken-route\" is invalid") {
		t.Fatalf("expected invalid persisted route error, got %v", err)
	}
	if got := router.Snapshot(); len(got) != 1 || got[0].ID != existing.ID {
		t.Fatalf("expected existing route snapshot to remain published, got %+v", got)
	}
}

func TestBootstrapDynamicConfigRejectsIncompatibleDefaultsBeforeSeeding(t *testing.T) {
	cipher, err := configcrypto.NewFromString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	if err != nil {
		t.Fatalf("create config cipher: %v", err)
	}
	cfg := config.Config{
		Logging: config.LoggingConfig{Format: "text"},
		Database: config.DatabaseConfig{
			Path:               t.TempDir() + "/gateway.db",
			BusyTimeout:        time.Second,
			MaxOpenConnections: 1,
		},
		Destinations: map[string]config.DestinationConfig{
			"slack-default": {Type: domain.DestinationSlack, ResolvedURL: "https://example.invalid/default"},
		},
		Routes: []config.RouteConfig{{
			ID:           "default-route",
			Destinations: []string{"slack-default"},
		}},
	}
	store, err := sqlite.OpenWithCipher(cfg.Database, observability.NewLogger(cfg.Logging), cipher)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.CreateDestination(context.Background(), domain.ManagedDestination{
		ID:         "slack-operator",
		Type:       domain.DestinationSlack,
		WebhookURL: "https://example.invalid/operator",
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create operator destination: %v", err)
	}

	svc := NewService(
		cfg,
		clock.Real{},
		store,
		nil,
		routing.New(nil),
		runtimeconfig.NewIntegrationRegistry(nil),
		runtimeconfig.NewDestinationRegistry(nil),
		runtimeconfig.NewRendererProfileRegistry(nil),
	)
	err = svc.BootstrapDynamicConfig(context.Background())
	if err == nil || !strings.Contains(err.Error(), "route references unknown destination slack-default") {
		t.Fatalf("expected incompatible startup defaults error, got %v", err)
	}
	routes, err := store.ListRoutes(context.Background())
	if err != nil {
		t.Fatalf("list routes after rejected seed: %v", err)
	}
	if len(routes) != 0 {
		t.Fatalf("expected invalid defaults not to be seeded, got %+v", routes)
	}
}

func TestCreateRouteReturnsRuntimeReloadRequiredWhenStoreWriteSucceedsButReloadFails(t *testing.T) {
	store := &reloadFailingRouteStoreStub{listRoutesErr: errors.New("reload failed")}
	svc := NewService(config.Config{
		API: config.APIConfig{
			ResolvedAdminToken: "admin-secret",
		},
	}, clock.Real{}, store, nil, routing.New(nil), runtimeconfig.NewIntegrationRegistry(nil), runtimeconfig.NewDestinationRegistry(map[string]config.DestinationConfig{
		"slack-deployments": {
			Type: domain.DestinationSlack,
		},
	}), runtimeconfig.NewRendererProfileRegistry(nil))

	_, err := svc.CreateRoute(context.Background(), domain.Route{
		ID:           "watcher-all-events",
		Match:        domain.RouteMatchCriteria{Sources: []domain.Source{domain.SourceWatcher}},
		Destinations: []string{"slack-deployments"},
	})
	if !errors.Is(err, ErrRuntimeReloadRequired) {
		t.Fatalf("expected runtime reload required error, got %v", err)
	}
	if !store.createCalled {
		t.Fatal("expected route create to persist before reload failure")
	}
}

func newTestService() *Service {
	return NewService(config.Config{
		API: config.APIConfig{
			ResolvedAdminToken: "admin-secret",
		},
		Destinations: map[string]config.DestinationConfig{
			"slack-deployments": {
				Type: domain.DestinationSlack,
			},
		},
	}, clock.Real{}, &routeStoreStub{}, nil, routing.New(nil), runtimeconfig.NewIntegrationRegistry(nil), runtimeconfig.NewDestinationRegistry(map[string]config.DestinationConfig{
		"slack-deployments": {
			Type: domain.DestinationSlack,
		},
	}), runtimeconfig.NewRendererProfileRegistry(nil))
}

type routeStoreStub struct{}

func (routeStoreStub) SeedDynamicConfig(context.Context, []domain.ManagedIntegration, []domain.ManagedDestination, []domain.ManagedRendererProfile, []domain.Route) error {
	return nil
}
func (routeStoreStub) HasActiveDeliveriesForDestination(context.Context, string) (bool, error) {
	return false, nil
}
func (routeStoreStub) ListIntegrations(context.Context) ([]domain.ManagedIntegration, error) {
	return nil, nil
}
func (routeStoreStub) GetIntegration(context.Context, string) (domain.ManagedIntegration, error) {
	return domain.ManagedIntegration{}, context.Canceled
}
func (routeStoreStub) CreateIntegration(context.Context, domain.ManagedIntegration) error { return nil }
func (routeStoreStub) UpdateIntegration(context.Context, domain.ManagedIntegration) error { return nil }
func (routeStoreStub) DeleteIntegration(context.Context, string) error                    { return nil }
func (routeStoreStub) ListDestinations(context.Context) ([]domain.ManagedDestination, error) {
	return nil, nil
}
func (routeStoreStub) GetDestination(context.Context, string) (domain.ManagedDestination, error) {
	return domain.ManagedDestination{}, context.Canceled
}
func (routeStoreStub) CreateDestination(context.Context, domain.ManagedDestination) error { return nil }
func (routeStoreStub) UpdateDestination(context.Context, domain.ManagedDestination) error { return nil }
func (routeStoreStub) DeleteDestination(context.Context, string) error                    { return nil }
func (routeStoreStub) ListRendererProfiles(context.Context) ([]domain.ManagedRendererProfile, error) {
	return nil, nil
}
func (routeStoreStub) GetRendererProfile(context.Context, string) (domain.ManagedRendererProfile, error) {
	return domain.ManagedRendererProfile{}, context.Canceled
}
func (routeStoreStub) CreateRendererProfile(context.Context, domain.ManagedRendererProfile) error {
	return nil
}
func (routeStoreStub) UpdateRendererProfile(context.Context, domain.ManagedRendererProfile) error {
	return nil
}
func (routeStoreStub) DeleteRendererProfile(context.Context, string) error { return nil }
func (routeStoreStub) ListRoutes(context.Context) ([]domain.Route, error)  { return nil, nil }
func (routeStoreStub) GetRoute(context.Context, string) (domain.Route, error) {
	return domain.Route{}, context.Canceled
}
func (routeStoreStub) CreateRoute(context.Context, domain.Route) error { return nil }
func (routeStoreStub) UpdateRoute(context.Context, domain.Route) error { return nil }
func (routeStoreStub) DeleteRoute(context.Context, string) error       { return nil }
func (routeStoreStub) GetReceipt(context.Context, string) (domain.Receipt, error) {
	return domain.Receipt{}, context.Canceled
}
func (routeStoreStub) ListReceipts(context.Context, domain.ReceiptFilter) (domain.ReceiptPage, error) {
	return domain.ReceiptPage{}, nil
}
func (routeStoreStub) GetDelivery(context.Context, string) (domain.Delivery, error) {
	return domain.Delivery{}, context.Canceled
}
func (routeStoreStub) ListEventsByReceipt(context.Context, string) ([]domain.Event, error) {
	return nil, nil
}
func (routeStoreStub) ListDeliveriesByEventIDs(context.Context, []string) (map[string][]domain.Delivery, error) {
	return map[string][]domain.Delivery{}, nil
}
func (routeStoreStub) ListDeliveries(context.Context, domain.DeliveryFilter) (domain.DeliveryPage, error) {
	return domain.DeliveryPage{}, nil
}
func (routeStoreStub) RetryDelivery(context.Context, string, time.Time) error { return nil }
func (routeStoreStub) GetEvent(context.Context, string) (domain.Event, error) {
	return domain.Event{}, context.Canceled
}
func (routeStoreStub) Ping(context.Context) error { return nil }

type seedStoreStub struct {
	routeStoreStub
	integrations []domain.ManagedIntegration
	destinations []domain.ManagedDestination
	profiles     []domain.ManagedRendererProfile
	routes       []domain.Route
}

type activeDeliveryStoreStub struct {
	seedStoreStub
}

func (*activeDeliveryStoreStub) HasActiveDeliveriesForDestination(context.Context, string) (bool, error) {
	return true, nil
}

type reloadFailingRouteStoreStub struct {
	routeStoreStub
	createCalled  bool
	listRoutesErr error
}

func (s *seedStoreStub) SeedDynamicConfig(_ context.Context, integrations []domain.ManagedIntegration, destinations []domain.ManagedDestination, profiles []domain.ManagedRendererProfile, routes []domain.Route) error {
	if len(s.integrations) == 0 {
		s.integrations = append([]domain.ManagedIntegration(nil), integrations...)
	}
	if len(s.destinations) == 0 {
		s.destinations = append([]domain.ManagedDestination(nil), destinations...)
	}
	if len(s.profiles) == 0 {
		s.profiles = append([]domain.ManagedRendererProfile(nil), profiles...)
	}
	if len(s.routes) == 0 {
		s.routes = append([]domain.Route(nil), routes...)
	}
	return nil
}

func (s *seedStoreStub) ListRoutes(context.Context) ([]domain.Route, error) {
	return append([]domain.Route(nil), s.routes...), nil
}

func (s *reloadFailingRouteStoreStub) CreateRoute(_ context.Context, route domain.Route) error {
	s.createCalled = true
	return nil
}

func (s *reloadFailingRouteStoreStub) ListRoutes(context.Context) ([]domain.Route, error) {
	return nil, s.listRoutesErr
}

func (s *seedStoreStub) ListIntegrations(context.Context) ([]domain.ManagedIntegration, error) {
	return append([]domain.ManagedIntegration(nil), s.integrations...), nil
}
func (s *seedStoreStub) CreateIntegration(_ context.Context, integration domain.ManagedIntegration) error {
	s.integrations = append(s.integrations, integration)
	return nil
}
func (s *seedStoreStub) GetIntegration(_ context.Context, id string) (domain.ManagedIntegration, error) {
	for _, item := range s.integrations {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.ManagedIntegration{}, errors.New("not found")
}
func (s *seedStoreStub) UpdateIntegration(_ context.Context, integration domain.ManagedIntegration) error {
	for i, item := range s.integrations {
		if item.ID == integration.ID {
			s.integrations[i] = integration
			return nil
		}
	}
	return errors.New("not found")
}
func (s *seedStoreStub) DeleteIntegration(_ context.Context, id string) error {
	for i, item := range s.integrations {
		if item.ID == id {
			s.integrations = append(s.integrations[:i], s.integrations[i+1:]...)
			return nil
		}
	}
	return nil
}
func (s *seedStoreStub) ListDestinations(context.Context) ([]domain.ManagedDestination, error) {
	return append([]domain.ManagedDestination(nil), s.destinations...), nil
}
func (s *seedStoreStub) CreateDestination(_ context.Context, destination domain.ManagedDestination) error {
	s.destinations = append(s.destinations, destination)
	return nil
}
func (s *seedStoreStub) GetDestination(_ context.Context, id string) (domain.ManagedDestination, error) {
	for _, item := range s.destinations {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.ManagedDestination{}, errors.New("not found")
}
func (s *seedStoreStub) UpdateDestination(_ context.Context, destination domain.ManagedDestination) error {
	for i, item := range s.destinations {
		if item.ID == destination.ID {
			s.destinations[i] = destination
			return nil
		}
	}
	return errors.New("not found")
}
func (s *seedStoreStub) DeleteDestination(_ context.Context, id string) error {
	for i, item := range s.destinations {
		if item.ID == id {
			s.destinations = append(s.destinations[:i], s.destinations[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *seedStoreStub) ListRendererProfiles(context.Context) ([]domain.ManagedRendererProfile, error) {
	return append([]domain.ManagedRendererProfile(nil), s.profiles...), nil
}
func (s *seedStoreStub) CreateRendererProfile(_ context.Context, profile domain.ManagedRendererProfile) error {
	s.profiles = append(s.profiles, profile)
	return nil
}
func (s *seedStoreStub) GetRendererProfile(_ context.Context, id string) (domain.ManagedRendererProfile, error) {
	for _, item := range s.profiles {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.ManagedRendererProfile{}, errors.New("not found")
}
func (s *seedStoreStub) UpdateRendererProfile(_ context.Context, profile domain.ManagedRendererProfile) error {
	for i, item := range s.profiles {
		if item.ID == profile.ID {
			s.profiles[i] = profile
			return nil
		}
	}
	return errors.New("not found")
}
func (s *seedStoreStub) DeleteRendererProfile(_ context.Context, id string) error {
	for i, item := range s.profiles {
		if item.ID == id {
			s.profiles = append(s.profiles[:i], s.profiles[i+1:]...)
			return nil
		}
	}
	return nil
}
func (s *seedStoreStub) ListDeliveriesByEventIDs(_ context.Context, eventIDs []string) (map[string][]domain.Delivery, error) {
	out := make(map[string][]domain.Delivery, len(eventIDs))
	for _, eventID := range eventIDs {
		out[eventID] = nil
	}
	return out, nil
}
