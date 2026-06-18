package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/iweka-dev/webhook-hub/internal/clock"
	"github.com/iweka-dev/webhook-hub/internal/config"
	"github.com/iweka-dev/webhook-hub/internal/domain"
	"github.com/iweka-dev/webhook-hub/internal/routing"
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
	}, clock.Real{}, &routeStoreStub{}, nil, routing.New(nil))
}

type routeStoreStub struct{}

func (routeStoreStub) ListRoutes(context.Context) ([]domain.Route, error) { return nil, nil }
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
