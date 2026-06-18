package routing

import (
	"testing"

	"github.com/iweka-dev/webhook-hub/internal/domain"
)

func TestEngineDestinationsDedupes(t *testing.T) {
	engine := New([]domain.Route{
		{ID: "a", Match: domain.RouteMatchCriteria{Types: []string{"watcher.deployment.failed"}}, Destinations: []string{"slack-a"}},
		{ID: "b", Match: domain.RouteMatchCriteria{Sources: []domain.Source{domain.SourceWatcher}}, Destinations: []string{"slack-a", "slack-b"}},
	})

	matches := engine.Destinations(domain.Event{
		ID:     "evt1",
		Source: domain.SourceWatcher,
		Type:   "watcher.deployment.failed",
	})

	if len(matches) != 2 {
		t.Fatalf("expected 2 unique matches, got %d", len(matches))
	}
}
