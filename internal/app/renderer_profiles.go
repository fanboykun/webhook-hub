package app

import (
	"context"
	"errors"

	"github.com/fanboykun/webhook-hub/internal/domain"
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
	if err := s.validateRendererProfile(profile); err != nil {
		return domain.ManagedRendererProfile{}, err
	}
	now := s.clock.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	if err := s.store.CreateRendererProfile(ctx, profile); err != nil {
		return domain.ManagedRendererProfile{}, err
	}
	if err := s.ReloadDynamicConfig(ctx); err != nil {
		return domain.ManagedRendererProfile{}, errors.Join(ErrRuntimeReloadRequired, err)
	}
	return profile, nil
}

func (s *Service) UpdateRendererProfile(ctx context.Context, profile domain.ManagedRendererProfile) (domain.ManagedRendererProfile, error) {
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
	if err := s.store.UpdateRendererProfile(ctx, profile); err != nil {
		return domain.ManagedRendererProfile{}, err
	}
	if err := s.ReloadDynamicConfig(ctx); err != nil {
		return domain.ManagedRendererProfile{}, errors.Join(ErrRuntimeReloadRequired, err)
	}
	return profile, nil
}

func (s *Service) DeleteRendererProfile(ctx context.Context, profileID string) error {
	if _, err := s.store.GetRendererProfile(ctx, profileID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRendererProfileNotFound
		}
		return err
	}
	if err := s.store.DeleteRendererProfile(ctx, profileID); err != nil {
		return err
	}
	if err := s.ReloadDynamicConfig(ctx); err != nil {
		return errors.Join(ErrRuntimeReloadRequired, err)
	}
	return nil
}
