package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"gorm.io/gorm"
)

func (s *Service) ListRendererProfiles(ctx context.Context) ([]domain.ManagedRendererProfile, error) {
	return s.store.ListRendererProfiles(ctx)
}

func (s *Service) GetRendererProfile(ctx context.Context, profileID string) (domain.ManagedRendererProfile, error) {
	item, err := s.store.GetRendererProfile(ctx, profileID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ManagedRendererProfile{}, ErrRendererProfileNotFound
		}
		return domain.ManagedRendererProfile{}, err
	}
	return item, nil
}

func (s *Service) CreateRendererProfile(ctx context.Context, profile domain.ManagedRendererProfile) (domain.ManagedRendererProfile, error) {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	if err := s.validateRendererProfile(profile); err != nil {
		return domain.ManagedRendererProfile{}, err
	}
	now := s.clock.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	if err := s.store.CreateRendererProfile(ctx, profile); err != nil {
		return domain.ManagedRendererProfile{}, err
	}
	if err := s.reloadDynamicConfigLocked(ctx); err != nil {
		return domain.ManagedRendererProfile{}, errors.Join(ErrRuntimeReloadRequired, err)
	}
	return profile, nil
}

func (s *Service) UpdateRendererProfile(ctx context.Context, profile domain.ManagedRendererProfile) (domain.ManagedRendererProfile, error) {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	current, err := s.store.GetRendererProfile(ctx, profile.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ManagedRendererProfile{}, ErrRendererProfileNotFound
		}
		return domain.ManagedRendererProfile{}, err
	}
	profile.CreatedAt = current.CreatedAt
	profile.UpdatedAt = s.clock.Now().UTC()
	if err := s.validateRendererProfile(profile); err != nil {
		return domain.ManagedRendererProfile{}, err
	}
	if err := s.validateRendererProfileUpdate(ctx, profile); err != nil {
		return domain.ManagedRendererProfile{}, err
	}
	if err := s.store.UpdateRendererProfile(ctx, profile); err != nil {
		return domain.ManagedRendererProfile{}, err
	}
	if err := s.reloadDynamicConfigLocked(ctx); err != nil {
		return domain.ManagedRendererProfile{}, errors.Join(ErrRuntimeReloadRequired, err)
	}
	return profile, nil
}

func (s *Service) DeleteRendererProfile(ctx context.Context, profileID string) error {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	if _, err := s.store.GetRendererProfile(ctx, profileID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRendererProfileNotFound
		}
		return err
	}
	destinations, err := s.store.ListDestinations(ctx)
	if err != nil {
		return err
	}
	for _, destination := range destinations {
		for _, referencedID := range destination.RendererProfiles {
			if referencedID == profileID {
				return fmt.Errorf("%w: destination %q references renderer profile %q", ErrRendererProfileInUse, destination.ID, profileID)
			}
		}
	}
	if err := s.store.DeleteRendererProfile(ctx, profileID); err != nil {
		return err
	}
	if err := s.reloadDynamicConfigLocked(ctx); err != nil {
		return errors.Join(ErrRuntimeReloadRequired, err)
	}
	return nil
}

func (s *Service) validateRendererProfileUpdate(ctx context.Context, updated domain.ManagedRendererProfile) error {
	profiles, err := s.store.ListRendererProfiles(ctx)
	if err != nil {
		return err
	}
	profileMap := runtimeconfig.MapRendererProfiles(profiles)
	profileMap[updated.ID] = updated.Profile
	temporary := runtimeconfig.NewRendererProfileRegistry(nil)
	if err := temporary.Replace(profileMap); err != nil {
		return err
	}
	destinations, err := s.store.ListDestinations(ctx)
	if err != nil {
		return err
	}
	for _, destination := range destinations {
		if err := s.validateDestinationWithRegistry(destination, temporary); err != nil {
			return fmt.Errorf("renderer profile update invalidates destination %q: %w", destination.ID, err)
		}
	}
	return nil
}
