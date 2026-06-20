package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/clock"
	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/routing"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
)

func TestCreateRouteNormalizesBlankSelectorValues(t *testing.T) {
	svc := newTestService()

	route, err := svc.CreateRoute(context.Background(), "Bearer admin-secret", domain.Route{
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

	_, err := svc.CreateRoute(context.Background(), "Bearer admin-secret", domain.Route{
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

	route, err := svc.CreateRoute(context.Background(), "Bearer admin-secret", domain.Route{
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

	_, err := svc.CreateRoute(context.Background(), "Bearer admin-secret", domain.Route{
		ID:           "watcher-all-events",
		Destinations: []string{"slack-deployments"},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown destination") {
		t.Fatalf("expected unknown destination error, got %v", err)
	}
}

func TestBootstrapDynamicConfigSeedsAndReloadsRegistries(t *testing.T) {
	store := &seedStoreStub{}
	integrations := runtimeconfig.NewIntegrationRegistry(nil)
	destinations := runtimeconfig.NewDestinationRegistry(nil)
	profiles := runtimeconfig.NewRendererProfileRegistry(nil)
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
			"detailed": {
				"watcher": {
					Default: config.DestinationTemplates{
						Slack: &config.SlackTemplateConfig{
							Title: "{{.Title}}",
							Body:  "{{.Summary}}",
						},
					},
				},
			},
		},
	}, clock.Real{}, store, nil, routing.New(nil), integrations, destinations, profiles)

	if err := svc.BootstrapDynamicConfig(context.Background()); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	if _, ok := integrations.Get("github-main"); !ok {
		t.Fatal("expected integration registry to be seeded")
	}
	if _, ok := destinations.Get("slack-deployments"); !ok {
		t.Fatal("expected destination registry to be seeded")
	}
	if _, ok := profiles.Get("detailed"); !ok {
		t.Fatal("expected renderer profile registry to be seeded")
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
func (routeStoreStub) ListDeliveries(context.Context, domain.DeliveryFilter) (domain.DeliveryPage, error) {
	return domain.DeliveryPage{}, nil
}
func (routeStoreStub) RetryDelivery(context.Context, string, time.Time) error { return nil }
func (routeStoreStub) Ping(context.Context) error                             { return nil }

type seedStoreStub struct {
	routeStoreStub
	integrations []domain.ManagedIntegration
	destinations []domain.ManagedDestination
	profiles     []domain.ManagedRendererProfile
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
