package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"github.com/fanboykun/webhook-hub/internal/storage/sqlite"
)

type dynamicConfigSnapshot struct {
	integrations []domain.ManagedIntegration
	destinations []domain.ManagedDestination
	profiles     []domain.ManagedRendererProfile
	routes       []domain.Route
}

func (s *Service) BootstrapDynamicConfig(ctx context.Context) error {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()

	now := s.clock.Now().UTC()
	integrations := runtimeconfig.StaticIntegrations(s.cfg)
	for i := range integrations {
		integrations[i].CreatedAt = now
		integrations[i].UpdatedAt = now
	}
	destinations := runtimeconfig.StaticDestinations(s.cfg)
	for i := range destinations {
		destinations[i].CreatedAt = now
		destinations[i].UpdatedAt = now
	}
	profiles := runtimeconfig.StaticRendererProfiles(s.cfg)
	for i := range profiles {
		profiles[i].CreatedAt = now
		profiles[i].UpdatedAt = now
	}
	routes := runtimeconfig.StaticRoutes(s.cfg)
	for i := range routes {
		routes[i].CreatedAt = now
		routes[i].UpdatedAt = now
	}
	defaults := dynamicConfigSnapshot{
		integrations: integrations,
		destinations: destinations,
		profiles:     profiles,
		routes:       routes,
	}
	persisted, err := s.readDynamicConfigSnapshot(ctx)
	if err != nil {
		return err
	}
	candidate := persisted.withDefaults(defaults)
	if err := s.validateDynamicConfigSnapshot(candidate); err != nil {
		return fmt.Errorf("effective startup configuration is invalid: %w", err)
	}
	if err := s.store.SeedDynamicConfig(ctx, integrations, destinations, profiles, routes); err != nil {
		return err
	}
	return s.reloadDynamicConfigLocked(ctx)
}

func (s *Service) ReloadDynamicConfig(ctx context.Context) error {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	return s.reloadDynamicConfigLocked(ctx)
}

func (s *Service) reloadDynamicConfigLocked(ctx context.Context) error {
	snapshot, err := s.readDynamicConfigSnapshot(ctx)
	if err != nil {
		return err
	}
	if err := s.validateDynamicConfigSnapshot(snapshot); err != nil {
		return err
	}
	if s.profiles != nil {
		if err := s.profiles.Replace(runtimeconfig.MapRendererProfiles(snapshot.profiles)); err != nil {
			return err
		}
	}
	if s.integrations != nil {
		s.integrations.Replace(runtimeconfig.MapIntegrations(snapshot.integrations))
	}
	if s.destinations != nil {
		s.destinations.Replace(runtimeconfig.MapDestinations(snapshot.destinations))
	}
	s.router.Replace(snapshot.routes)
	return nil
}

func (s *Service) readDynamicConfigSnapshot(ctx context.Context) (dynamicConfigSnapshot, error) {
	integrations, err := s.store.ListIntegrations(ctx)
	if err != nil {
		return dynamicConfigSnapshot{}, err
	}
	destinations, err := s.store.ListDestinations(ctx)
	if err != nil {
		return dynamicConfigSnapshot{}, err
	}
	profiles, err := s.store.ListRendererProfiles(ctx)
	if err != nil {
		return dynamicConfigSnapshot{}, err
	}
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return dynamicConfigSnapshot{}, err
	}
	return dynamicConfigSnapshot{
		integrations: integrations,
		destinations: destinations,
		profiles:     profiles,
		routes:       routes,
	}, nil
}

func (s dynamicConfigSnapshot) withDefaults(defaults dynamicConfigSnapshot) dynamicConfigSnapshot {
	if len(s.integrations) == 0 {
		s.integrations = defaults.integrations
	}
	if len(s.destinations) == 0 {
		s.destinations = defaults.destinations
	}
	if len(s.profiles) == 0 {
		s.profiles = defaults.profiles
	}
	if len(s.routes) == 0 {
		s.routes = defaults.routes
	}
	return s
}

func (s *Service) validateDynamicConfigSnapshot(snapshot dynamicConfigSnapshot) error {
	for _, integration := range snapshot.integrations {
		if err := s.validateIntegration(integration); err != nil {
			return fmt.Errorf("persisted integration %q is invalid: %w", integration.ID, err)
		}
	}
	for _, profile := range snapshot.profiles {
		if err := s.validateRendererProfile(profile); err != nil {
			return fmt.Errorf("persisted renderer profile %q is invalid: %w", profile.ID, err)
		}
	}
	temporaryProfiles := runtimeconfig.NewRendererProfileRegistry(nil)
	if err := temporaryProfiles.Replace(runtimeconfig.MapRendererProfiles(snapshot.profiles)); err != nil {
		return err
	}
	for _, destination := range snapshot.destinations {
		if err := s.validateDestinationWithRegistry(destination, temporaryProfiles); err != nil {
			return fmt.Errorf("persisted destination %q is invalid: %w", destination.ID, err)
		}
	}
	temporaryDestinations := runtimeconfig.NewDestinationRegistry(runtimeconfig.MapDestinations(snapshot.destinations))
	for _, route := range snapshot.routes {
		if err := validateRouteWithDestinations(route, temporaryDestinations); err != nil {
			return fmt.Errorf("persisted route %q is invalid: %w", route.ID, err)
		}
	}
	return nil
}

func normalizeDynamicConfigErr(err error) error {
	if errors.Is(err, sqlite.ErrEncryptionUnavailable) {
		return ErrDynamicConfigUnavailable
	}
	return err
}

func (s *Service) LoadRoutes(ctx context.Context) error {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	return s.loadRoutesLocked(ctx)
}

func (s *Service) loadRoutesLocked(ctx context.Context) error {
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return err
	}
	for _, route := range routes {
		if err := s.validateRoute(route); err != nil {
			return fmt.Errorf("persisted route %q is invalid: %w", route.ID, err)
		}
	}
	s.router.Replace(routes)
	return nil
}
