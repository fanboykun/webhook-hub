package storage

import (
	"context"
	"time"

	"github.com/fanboykun/webhook-hub/internal/domain"
)

type Store interface {
	Ingest(ctx context.Context, batch domain.IngestBatch) (domain.IngestResult, error)
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
	ListRendererProfiles(ctx context.Context) ([]domain.ManagedRendererProfile, error)
	GetRendererProfile(ctx context.Context, id string) (domain.ManagedRendererProfile, error)
	CreateRendererProfile(ctx context.Context, profile domain.ManagedRendererProfile) error
	UpdateRendererProfile(ctx context.Context, profile domain.ManagedRendererProfile) error
	DeleteRendererProfile(ctx context.Context, id string) error
	ListRoutes(ctx context.Context) ([]domain.Route, error)
	GetRoute(ctx context.Context, id string) (domain.Route, error)
	CreateRoute(ctx context.Context, route domain.Route) error
	UpdateRoute(ctx context.Context, route domain.Route) error
	DeleteRoute(ctx context.Context, id string) error
	GetReceipt(ctx context.Context, id string) (domain.Receipt, error)
	ListReceipts(ctx context.Context, filter domain.ReceiptFilter) (domain.ReceiptPage, error)
	GetEvent(ctx context.Context, id string) (domain.Event, error)
	ListEventsByReceipt(ctx context.Context, receiptID string) ([]domain.Event, error)
	ListDeliveries(ctx context.Context, filter domain.DeliveryFilter) (domain.DeliveryPage, error)
	GetDelivery(ctx context.Context, id string) (domain.Delivery, error)
	ClaimDueDeliveries(ctx context.Context, claim domain.ClaimRequest) ([]domain.DeliveryEnvelope, error)
	CompleteAttempt(ctx context.Context, result domain.AttemptResult) error
	RecoverExpiredLeases(ctx context.Context, now time.Time) (int64, error)
	RetryDelivery(ctx context.Context, id string, now time.Time) error
	Ping(ctx context.Context) error
	Close() error
}
