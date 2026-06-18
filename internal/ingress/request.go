package ingress

import (
	"context"
	"net/http"
	"net/netip"
	"time"

	"github.com/iweka-dev/webhook-hub/internal/config"
	"github.com/iweka-dev/webhook-hub/internal/domain"
)

type InboundRequest struct {
	IntegrationID string
	Headers       http.Header
	RawBody       []byte
	ReceivedAt    time.Time
	RemoteIP      netip.Addr
}

type AdapterResult struct {
	SourceDeliveryID string
	SourceEventType  string
	Events           []domain.Event
	IgnoreReason     string
}

type SourceAdapter interface {
	Source() domain.Source
	Verify(ctx context.Context, integration config.IntegrationConfig, req InboundRequest) error
	Normalize(ctx context.Context, integrationID string, integration config.IntegrationConfig, req InboundRequest) (AdapterResult, error)
}
