package app

import (
	"context"
	"errors"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"gorm.io/gorm"
)

func (s *Service) GetDelivery(ctx context.Context, deliveryID string) (domain.Delivery, error) {
	delivery, err := s.store.GetDelivery(ctx, deliveryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Delivery{}, ErrDeliveryNotFound
		}
		return domain.Delivery{}, err
	}
	return delivery, nil
}

func (s *Service) GetReceipt(ctx context.Context, receiptID string) (domain.ReceiptDetail, error) {
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

	eventIDs := make([]string, 0, len(events))
	for _, event := range events {
		eventIDs = append(eventIDs, event.ID)
	}
	deliveries, err := s.store.ListDeliveriesByEventIDs(ctx, eventIDs)
	if err != nil {
		return domain.ReceiptDetail{}, err
	}

	return domain.ReceiptDetail{
		Receipt:    receipt,
		Events:     events,
		Deliveries: deliveries,
	}, nil
}

func (s *Service) ListReceipts(ctx context.Context, filter domain.ReceiptFilter) (domain.ReceiptPage, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	return s.store.ListReceipts(ctx, filter)
}

func (s *Service) ListDeliveries(ctx context.Context, filter domain.DeliveryFilter) (domain.DeliveryPage, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	return s.store.ListDeliveries(ctx, filter)
}

func (s *Service) RetryDelivery(ctx context.Context, deliveryID string) error {
	return s.store.RetryDelivery(ctx, deliveryID, s.clock.Now().UTC())
}
