package defaults

import (
	"sync"

	"github.com/fanboykun/webhook-hub/internal/eventcatalog"
	ghcatalog "github.com/fanboykun/webhook-hub/internal/eventcatalog/github"
	watchercatalog "github.com/fanboykun/webhook-hub/internal/eventcatalog/watcher"
)

var (
	once     sync.Once
	registry *eventcatalog.Registry
)

func Registry() *eventcatalog.Registry {
	once.Do(func() {
		definitions := append([]eventcatalog.Definition{}, watchercatalog.Definitions()...)
		definitions = append(definitions, ghcatalog.Definitions()...)
		registry = eventcatalog.MustRegistry(definitions...)
	})
	return registry
}
