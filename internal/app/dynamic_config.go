package app

import (
	"context"
	"errors"

	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"github.com/fanboykun/webhook-hub/internal/storage/sqlite"
)

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

	profiles, err := s.store.ListRendererProfiles(ctx)
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		for _, profile := range runtimeconfig.StaticRendererProfiles(s.cfg) {
			profile.CreatedAt = now
			profile.UpdatedAt = now
			if err := s.store.CreateRendererProfile(ctx, profile); err != nil {
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
	profiles, err := s.store.ListRendererProfiles(ctx)
	if err != nil {
		return err
	}
	if s.integrations != nil {
		s.integrations.Replace(runtimeconfig.MapIntegrations(integrations))
	}
	if s.destinations != nil {
		s.destinations.Replace(runtimeconfig.MapDestinations(destinations))
	}
	if s.profiles != nil {
		s.profiles.Replace(runtimeconfig.MapRendererProfiles(profiles))
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
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return err
	}
	s.router.Replace(routes)
	return nil
}
