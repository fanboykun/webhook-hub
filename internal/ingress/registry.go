package ingress

import "github.com/fanboykun/webhook-hub/internal/domain"

type Registry struct {
	adapters map[domain.Source]SourceAdapter
}

func NewRegistry(adapters ...SourceAdapter) *Registry {
	out := &Registry{adapters: make(map[domain.Source]SourceAdapter, len(adapters))}
	for _, adapter := range adapters {
		out.adapters[adapter.Source()] = adapter
	}
	return out
}

func (r *Registry) Get(source domain.Source) (SourceAdapter, bool) {
	adapter, ok := r.adapters[source]
	return adapter, ok
}
