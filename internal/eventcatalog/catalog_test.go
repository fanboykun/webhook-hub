package eventcatalog

import (
	"testing"

	"github.com/fanboykun/webhook-hub/internal/domain"
)

func TestRegistryRejectsDuplicateKeys(t *testing.T) {
	_, err := NewRegistry(
		Definition{Key: "watcher.deployment.failed", Source: domain.SourceWatcher, PayloadVersion: 1},
		Definition{Key: "watcher.deployment.failed", Source: domain.SourceWatcher, PayloadVersion: 1},
	)
	if err == nil {
		t.Fatal("expected duplicate key error")
	}
}
