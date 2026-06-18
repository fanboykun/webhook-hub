package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
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
	ErrReceiptNotFound  = errors.New("receipt not found")
)

type Store interface {
	ListRoutes(ctx context.Context) ([]domain.Route, error)
	GetRoute(ctx context.Context, id string) (domain.Route, error)
	CreateRoute(ctx context.Context, route domain.Route) error
	UpdateRoute(ctx context.Context, route domain.Route) error
	DeleteRoute(ctx context.Context, id string) error
	GetReceipt(ctx context.Context, id string) (domain.Receipt, error)
	ListReceipts(ctx context.Context, filter domain.ReceiptFilter) (domain.ReceiptPage, error)
	GetDelivery(ctx context.Context, id string) (domain.Delivery, error)
	ListEventsByReceipt(ctx context.Context, receiptID string) ([]domain.Event, error)
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

func (s *Service) GetReceipt(ctx context.Context, authorization, receiptID string) (domain.ReceiptDetail, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.ReceiptDetail{}, err
	}

	receipt, err := s.store.GetReceipt(ctx, receiptID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ReceiptDetail{}, ErrReceiptNotFound
		}
		return domain.ReceiptDetail{}, err
	}

	events, err := s.store.ListEventsByReceipt(ctx, receiptID)
	if err != nil {
		return domain.ReceiptDetail{}, err
	}

	deliveries := make(map[string][]domain.Delivery, len(events))
	for _, event := range events {
		page, err := s.store.ListDeliveries(ctx, domain.DeliveryFilter{EventID: event.ID, Limit: 100})
		if err != nil {
			return domain.ReceiptDetail{}, err
		}
		deliveries[event.ID] = page.Items
	}

	return domain.ReceiptDetail{
		Receipt:    receipt,
		Events:     events,
		Deliveries: deliveries,
	}, nil
}

func (s *Service) ListReceipts(ctx context.Context, authorization string, filter domain.ReceiptFilter) (domain.ReceiptPage, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.ReceiptPage{}, err
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	return s.store.ListReceipts(ctx, filter)
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
	if err := s.LoadRoutes(ctx); err != nil {
		return domain.Route{}, err
	}
	return route, nil
}

func (s *Service) UpdateRoute(ctx context.Context, authorization string, route domain.Route) (domain.Route, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.Route{}, err
	}
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
	if strings.TrimSpace(route.ID) == "" {
		return errors.New("route id is required")
	}
	if len(route.Destinations) == 0 {
		return errors.New("route destinations must not be empty")
	}
	seenDestinations := make(map[string]struct{}, len(route.Destinations))
	for _, destinationID := range route.Destinations {
		destinationID = strings.TrimSpace(destinationID)
		if destinationID == "" {
			return errors.New("route destinations must not contain blank values")
		}
		if _, exists := seenDestinations[destinationID]; exists {
			return fmt.Errorf("route destinations must be unique: %s", destinationID)
		}
		seenDestinations[destinationID] = struct{}{}
		if _, exists := s.cfg.Destinations[destinationID]; !exists {
			return errors.New("route references unknown destination " + destinationID)
		}
	}
	if err := validateRouteMatch(route.Match); err != nil {
		return err
	}
	return nil
}

func normalizeRoute(route domain.Route) domain.Route {
	route.Match.Sources = normalizeSources(route.Match.Sources)
	route.Match.Types = normalizeStrings(route.Match.Types)
	route.Match.Severities = normalizeSeverities(route.Match.Severities)
	route.Match.Environments = normalizeStrings(route.Match.Environments)
	route.Destinations = normalizeStrings(route.Destinations)
	return route
}

func normalizeSources(values []domain.Source) []domain.Source {
	out := make([]domain.Source, 0, len(values))
	for _, value := range values {
		value = domain.Source(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func normalizeSeverities(values []domain.Severity) []domain.Severity {
	out := make([]domain.Severity, 0, len(values))
	for _, value := range values {
		value = domain.Severity(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func validateRouteMatch(match domain.RouteMatchCriteria) error {
	if err := validateRouteSources(match.Sources); err != nil {
		return err
	}
	if err := validateRouteEventTypes(match.Types); err != nil {
		return err
	}
	if err := validateRouteSeverities(match.Severities); err != nil {
		return err
	}
	if err := validateRouteEnvironments(match.Environments); err != nil {
		return err
	}
	return nil
}

func validateRouteSources(values []domain.Source) error {
	seen := map[domain.Source]struct{}{}
	for _, value := range values {
		value = domain.Source(strings.TrimSpace(string(value)))
		if value == "" {
			return errors.New("route sources must not contain blank values")
		}
		if !domain.IsKnownSource(value) {
			return fmt.Errorf("route sources contains unknown value %q", value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("route sources must be unique: %s", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateRouteEventTypes(values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return errors.New("route types must not contain blank values")
		}
		if !domain.IsKnownEventType(value) {
			return fmt.Errorf("route types contains unknown value %q", value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("route types must be unique: %s", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateRouteSeverities(values []domain.Severity) error {
	seen := map[domain.Severity]struct{}{}
	for _, value := range values {
		value = domain.Severity(strings.TrimSpace(string(value)))
		if value == "" {
			return errors.New("route severities must not contain blank values")
		}
		if !domain.IsKnownSeverity(value) {
			return fmt.Errorf("route severities contains unknown value %q", value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("route severities must be unique: %s", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateRouteEnvironments(values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return errors.New("route environments must not contain blank values")
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("route environments must be unique: %s", value)
		}
		seen[value] = struct{}{}
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
