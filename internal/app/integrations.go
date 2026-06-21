package app

import (
	"context"
	"errors"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/storage/sqlite"
	"gorm.io/gorm"
)

func (s *Service) ListIntegrations(ctx context.Context, source domain.Source) ([]domain.ManagedIntegration, error) {
	items, err := s.store.ListIntegrations(ctx)
	if err != nil {
		return items, normalizeDynamicConfigErr(err)
	}
	if source == "" {
		return items, nil
	}
	filtered := make([]domain.ManagedIntegration, 0, len(items))
	for _, item := range items {
		if item.Source == source {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *Service) GetIntegration(ctx context.Context, integrationID string) (domain.ManagedIntegration, error) {
	item, err := s.store.GetIntegration(ctx, integrationID)
	if err != nil {
		if errors.Is(err, sqlite.ErrEncryptionUnavailable) {
			return domain.ManagedIntegration{}, ErrDynamicConfigUnavailable
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ManagedIntegration{}, ErrIntegrationNotFound
		}
		return domain.ManagedIntegration{}, err
	}
	return item, nil
}

func (s *Service) CreateIntegration(ctx context.Context, integration domain.ManagedIntegration) (domain.ManagedIntegration, error) {
	if err := s.validateIntegration(integration); err != nil {
		return domain.ManagedIntegration{}, err
	}
	now := s.clock.Now().UTC()
	integration.CreatedAt = now
	integration.UpdatedAt = now
	if err := s.store.CreateIntegration(ctx, integration); err != nil {
		return domain.ManagedIntegration{}, normalizeDynamicConfigErr(err)
	}
	if err := s.ReloadDynamicConfig(ctx); err != nil {
		return domain.ManagedIntegration{}, errors.Join(ErrRuntimeReloadRequired, normalizeDynamicConfigErr(err))
	}
	return integration, nil
}

func (s *Service) UpdateIntegration(ctx context.Context, integration domain.ManagedIntegration) (domain.ManagedIntegration, error) {
	current, err := s.store.GetIntegration(ctx, integration.ID)
	if err != nil {
		if errors.Is(err, sqlite.ErrEncryptionUnavailable) {
			return domain.ManagedIntegration{}, ErrDynamicConfigUnavailable
		}
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
		return domain.ManagedIntegration{}, normalizeDynamicConfigErr(err)
	}
	if err := s.ReloadDynamicConfig(ctx); err != nil {
		return domain.ManagedIntegration{}, errors.Join(ErrRuntimeReloadRequired, normalizeDynamicConfigErr(err))
	}
	return integration, nil
}

func (s *Service) DeleteIntegration(ctx context.Context, integrationID string) error {
	if _, err := s.store.GetIntegration(ctx, integrationID); err != nil {
		if errors.Is(err, sqlite.ErrEncryptionUnavailable) {
			return ErrDynamicConfigUnavailable
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrIntegrationNotFound
		}
		return err
	}
	if err := s.store.DeleteIntegration(ctx, integrationID); err != nil {
		return normalizeDynamicConfigErr(err)
	}
	if err := s.ReloadDynamicConfig(ctx); err != nil {
		return errors.Join(ErrRuntimeReloadRequired, normalizeDynamicConfigErr(err))
	}
	return nil
}
