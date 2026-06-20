package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fanboykun/webhook-hub/internal/clock"
	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/ingress"
	"github.com/fanboykun/webhook-hub/internal/routing"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"gorm.io/gorm"
)

var (
	ErrUnauthorized        = errors.New("admin unauthorized")
	ErrRouteNotFound       = errors.New("route not found")
	ErrDeliveryNotFound    = errors.New("delivery not found")
	ErrReceiptNotFound     = errors.New("receipt not found")
	ErrIntegrationNotFound = errors.New("integration not found")
	ErrDestinationNotFound = errors.New("destination not found")
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
	cfg          config.Config
	clock        clock.Clock
	store        Store
	webhook      WebhookService
	router       *routing.Engine
	integrations *runtimeconfig.IntegrationRegistry
	destinations *runtimeconfig.DestinationRegistry
}

func NewService(cfg config.Config, clk clock.Clock, store Store, webhook WebhookService, router *routing.Engine, integrations *runtimeconfig.IntegrationRegistry, destinations *runtimeconfig.DestinationRegistry) *Service {
	return &Service{
		cfg:          cfg,
		clock:        clk,
		store:        store,
		webhook:      webhook,
		router:       router,
		integrations: integrations,
		destinations: destinations,
	}
}

func (s *Service) BootstrapDynamicConfig(ctx context.Context) error {
	now := s.clock.Now().UTC()

	integrations, err := s.store.ListIntegrations(ctx)
	if err != nil {
		return err
	}
	if len(integrations) == 0 {
		for _, integration := range runtimeconfig.StaticIntegrations(s.cfg) {
			integration.CreatedAt = now
			integration.UpdatedAt = now
			if err := s.store.CreateIntegration(ctx, integration); err != nil {
				return err
			}
		}
	}

	destinations, err := s.store.ListDestinations(ctx)
	if err != nil {
		return err
	}
	if len(destinations) == 0 {
		for _, destination := range runtimeconfig.StaticDestinations(s.cfg) {
			destination.CreatedAt = now
			destination.UpdatedAt = now
			if err := s.store.CreateDestination(ctx, destination); err != nil {
				return err
			}
		}
	}

	return s.ReloadDynamicConfig(ctx)
}

func (s *Service) ReloadDynamicConfig(ctx context.Context) error {
	integrations, err := s.store.ListIntegrations(ctx)
	if err != nil {
		return err
	}
	destinations, err := s.store.ListDestinations(ctx)
	if err != nil {
		return err
	}
	if s.integrations != nil {
		s.integrations.Replace(runtimeconfig.MapIntegrations(integrations))
	}
	if s.destinations != nil {
		s.destinations.Replace(runtimeconfig.MapDestinations(destinations))
	}
	return nil
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
		if s.destinations == nil {
			return errors.New("destination registry is not configured")
		}
		if _, exists := s.destinations.Get(destinationID); !exists {
			return errors.New("route references unknown destination " + destinationID)
		}
	}
	if err := validateRouteMatch(route.Match); err != nil {
		return err
	}
	return nil
}

func (s *Service) ListIntegrations(ctx context.Context, authorization string) ([]domain.ManagedIntegration, error) {
	if err := s.authorize(authorization); err != nil {
		return nil, err
	}
	return s.store.ListIntegrations(ctx)
}

func (s *Service) GetIntegration(ctx context.Context, authorization, integrationID string) (domain.ManagedIntegration, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.ManagedIntegration{}, err
	}
	item, err := s.store.GetIntegration(ctx, integrationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ManagedIntegration{}, ErrIntegrationNotFound
		}
		return domain.ManagedIntegration{}, err
	}
	return item, nil
}

func (s *Service) CreateIntegration(ctx context.Context, authorization string, integration domain.ManagedIntegration) (domain.ManagedIntegration, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.ManagedIntegration{}, err
	}
	if err := s.validateIntegration(integration); err != nil {
		return domain.ManagedIntegration{}, err
	}
	now := s.clock.Now().UTC()
	integration.CreatedAt = now
	integration.UpdatedAt = now
	if err := s.store.CreateIntegration(ctx, integration); err != nil {
		return domain.ManagedIntegration{}, err
	}
	if err := s.ReloadDynamicConfig(ctx); err != nil {
		return domain.ManagedIntegration{}, err
	}
	return integration, nil
}

func (s *Service) UpdateIntegration(ctx context.Context, authorization string, integration domain.ManagedIntegration) (domain.ManagedIntegration, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.ManagedIntegration{}, err
	}
	current, err := s.store.GetIntegration(ctx, integration.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ManagedIntegration{}, ErrIntegrationNotFound
		}
		return domain.ManagedIntegration{}, err
	}
	integration.CreatedAt = current.CreatedAt
	integration.UpdatedAt = s.clock.Now().UTC()
	if integration.Secret == "" || integration.Secret == "[REDACTED]" {
		integration.Secret = current.Secret
	}
	if integration.ClientSecret == "" || integration.ClientSecret == "[REDACTED]" {
		integration.ClientSecret = current.ClientSecret
	}
	if err := s.validateIntegration(integration); err != nil {
		return domain.ManagedIntegration{}, err
	}
	if err := s.store.UpdateIntegration(ctx, integration); err != nil {
		return domain.ManagedIntegration{}, err
	}
	if err := s.ReloadDynamicConfig(ctx); err != nil {
		return domain.ManagedIntegration{}, err
	}
	return integration, nil
}

func (s *Service) DeleteIntegration(ctx context.Context, authorization, integrationID string) error {
	if err := s.authorize(authorization); err != nil {
		return err
	}
	if _, err := s.store.GetIntegration(ctx, integrationID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrIntegrationNotFound
		}
		return err
	}
	if err := s.store.DeleteIntegration(ctx, integrationID); err != nil {
		return err
	}
	return s.ReloadDynamicConfig(ctx)
}

func (s *Service) ListDestinations(ctx context.Context, authorization string) ([]domain.ManagedDestination, error) {
	if err := s.authorize(authorization); err != nil {
		return nil, err
	}
	return s.store.ListDestinations(ctx)
}

func (s *Service) GetDestination(ctx context.Context, authorization, destinationID string) (domain.ManagedDestination, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.ManagedDestination{}, err
	}
	item, err := s.store.GetDestination(ctx, destinationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ManagedDestination{}, ErrDestinationNotFound
		}
		return domain.ManagedDestination{}, err
	}
	return item, nil
}

func (s *Service) CreateDestination(ctx context.Context, authorization string, destination domain.ManagedDestination) (domain.ManagedDestination, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.ManagedDestination{}, err
	}
	if err := s.validateDestination(destination); err != nil {
		return domain.ManagedDestination{}, err
	}
	now := s.clock.Now().UTC()
	destination.CreatedAt = now
	destination.UpdatedAt = now
	if err := s.store.CreateDestination(ctx, destination); err != nil {
		return domain.ManagedDestination{}, err
	}
	if err := s.ReloadDynamicConfig(ctx); err != nil {
		return domain.ManagedDestination{}, err
	}
	return destination, nil
}

func (s *Service) UpdateDestination(ctx context.Context, authorization string, destination domain.ManagedDestination) (domain.ManagedDestination, error) {
	if err := s.authorize(authorization); err != nil {
		return domain.ManagedDestination{}, err
	}
	current, err := s.store.GetDestination(ctx, destination.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ManagedDestination{}, ErrDestinationNotFound
		}
		return domain.ManagedDestination{}, err
	}
	destination.CreatedAt = current.CreatedAt
	destination.UpdatedAt = s.clock.Now().UTC()
	if destination.WebhookURL == "" || destination.WebhookURL == "[REDACTED]" {
		destination.WebhookURL = current.WebhookURL
	}
	if destination.BotToken == "" || destination.BotToken == "[REDACTED]" {
		destination.BotToken = current.BotToken
	}
	if err := s.validateDestination(destination); err != nil {
		return domain.ManagedDestination{}, err
	}
	if err := s.store.UpdateDestination(ctx, destination); err != nil {
		return domain.ManagedDestination{}, err
	}
	if err := s.ReloadDynamicConfig(ctx); err != nil {
		return domain.ManagedDestination{}, err
	}
	return destination, nil
}

func (s *Service) DeleteDestination(ctx context.Context, authorization, destinationID string) error {
	if err := s.authorize(authorization); err != nil {
		return err
	}
	if _, err := s.store.GetDestination(ctx, destinationID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDestinationNotFound
		}
		return err
	}
	if err := s.store.DeleteDestination(ctx, destinationID); err != nil {
		return err
	}
	return s.ReloadDynamicConfig(ctx)
}

func normalizeRoute(route domain.Route) domain.Route {
	route.Match.Sources = normalizeSources(route.Match.Sources)
	route.Match.Types = normalizeStrings(route.Match.Types)
	route.Match.Severities = normalizeSeverities(route.Match.Severities)
	route.Match.Environments = normalizeStrings(route.Match.Environments)
	route.Destinations = normalizeStrings(route.Destinations)
	return route
}

func (s *Service) validateIntegration(integration domain.ManagedIntegration) error {
	if strings.TrimSpace(integration.ID) == "" {
		return errors.New("integration id is required")
	}
	switch integration.Source {
	case domain.SourceWatcher:
		if !strings.HasPrefix(integration.Secret, "whsec_") {
			return errors.New("watcher integration secret must start with whsec_")
		}
	case domain.SourceGitHub:
		if strings.TrimSpace(integration.Secret) == "" {
			return errors.New("github integration secret is required")
		}
	default:
		return fmt.Errorf("integration source %q is not supported in this slice", integration.Source)
	}
	if integration.ReplayWindow < 0 {
		return errors.New("integration replay window must be >= 0")
	}
	return nil
}

func (s *Service) validateDestination(destination domain.ManagedDestination) error {
	if strings.TrimSpace(destination.ID) == "" {
		return errors.New("destination id is required")
	}
	switch destination.Type {
	case domain.DestinationSlack:
		if strings.TrimSpace(destination.WebhookURL) == "" {
			return errors.New("slack destination webhook_url is required")
		}
	case domain.DestinationTelegram:
		if strings.TrimSpace(destination.BotToken) == "" {
			return errors.New("telegram destination bot_token is required")
		}
		if !config.IsTelegramChatID(destination.ChatID) {
			return errors.New("telegram destination chat_id is invalid")
		}
	default:
		return fmt.Errorf("destination type %q is not supported in this slice", destination.Type)
	}
	if destination.Profile != "" {
		if _, ok := s.cfg.RendererProfiles[destination.Profile]; !ok {
			return fmt.Errorf("destination references undefined renderer profile %q", destination.Profile)
		}
	}
	return nil
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
