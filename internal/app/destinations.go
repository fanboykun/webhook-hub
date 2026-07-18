package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/storage/sqlite"
	"gorm.io/gorm"
)

func (s *Service) ListDestinations(ctx context.Context, destinationType domain.DestinationType) ([]domain.ManagedDestination, error) {
	items, err := s.store.ListDestinations(ctx)
	if err != nil {
		return items, normalizeDynamicConfigErr(err)
	}
	if destinationType == "" {
		return items, nil
	}
	filtered := make([]domain.ManagedDestination, 0, len(items))
	for _, item := range items {
		if item.Type == destinationType {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *Service) GetDestination(ctx context.Context, destinationID string) (domain.ManagedDestination, error) {
	item, err := s.store.GetDestination(ctx, destinationID)
	if err != nil {
		if errors.Is(err, sqlite.ErrEncryptionUnavailable) {
			return domain.ManagedDestination{}, ErrDynamicConfigUnavailable
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ManagedDestination{}, ErrDestinationNotFound
		}
		return domain.ManagedDestination{}, err
	}
	return item, nil
}

func (s *Service) CreateDestination(ctx context.Context, destination domain.ManagedDestination) (domain.ManagedDestination, error) {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	if err := s.validateDestination(destination); err != nil {
		return domain.ManagedDestination{}, err
	}
	now := s.clock.Now().UTC()
	destination.CreatedAt = now
	destination.UpdatedAt = now
	if err := s.store.CreateDestination(ctx, destination); err != nil {
		return domain.ManagedDestination{}, normalizeDynamicConfigErr(err)
	}
	if err := s.reloadDynamicConfigLocked(ctx); err != nil {
		return domain.ManagedDestination{}, errors.Join(ErrRuntimeReloadRequired, normalizeDynamicConfigErr(err))
	}
	return destination, nil
}

func (s *Service) UpdateDestination(ctx context.Context, destination domain.ManagedDestination) (domain.ManagedDestination, error) {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	current, err := s.store.GetDestination(ctx, destination.ID)
	if err != nil {
		if errors.Is(err, sqlite.ErrEncryptionUnavailable) {
			return domain.ManagedDestination{}, ErrDynamicConfigUnavailable
		}
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
		return domain.ManagedDestination{}, normalizeDynamicConfigErr(err)
	}
	if err := s.reloadDynamicConfigLocked(ctx); err != nil {
		return domain.ManagedDestination{}, errors.Join(ErrRuntimeReloadRequired, normalizeDynamicConfigErr(err))
	}
	return destination, nil
}

func (s *Service) DeleteDestination(ctx context.Context, destinationID string) error {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	if _, err := s.store.GetDestination(ctx, destinationID); err != nil {
		if errors.Is(err, sqlite.ErrEncryptionUnavailable) {
			return ErrDynamicConfigUnavailable
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDestinationNotFound
		}
		return err
	}
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return err
	}
	for _, route := range routes {
		for _, referencedID := range route.Destinations {
			if referencedID == destinationID {
				return fmt.Errorf("%w: route %q references destination %q", ErrDestinationInUse, route.ID, destinationID)
			}
		}
	}
	active, err := s.store.HasActiveDeliveriesForDestination(ctx, destinationID)
	if err != nil {
		return err
	}
	if active {
		return fmt.Errorf("%w: destination %q has active deliveries", ErrDestinationInUse, destinationID)
	}
	if err := s.store.DeleteDestination(ctx, destinationID); err != nil {
		return normalizeDynamicConfigErr(err)
	}
	if err := s.reloadDynamicConfigLocked(ctx); err != nil {
		return errors.Join(ErrRuntimeReloadRequired, normalizeDynamicConfigErr(err))
	}
	return nil
}
