package eventcatalog

import (
	"fmt"
	"slices"
	"sort"

	"github.com/fanboykun/webhook-hub/internal/domain"
)

type Key string

type Field struct {
	Path        string
	Type        string
	Required    bool
	Description string
}

type Definition struct {
	Key            Key
	Source         domain.Source
	PayloadVersion int
	Summary        string
	TemplatePaths  []string
	Fields         []Field
}

type Registry struct {
	definitions map[Key]Definition
}

func NewRegistry(definitions ...Definition) (*Registry, error) {
	items := make(map[Key]Definition, len(definitions))
	for _, definition := range definitions {
		if definition.Key == "" {
			return nil, fmt.Errorf("event definition key is required")
		}
		if definition.Source == "" {
			return nil, fmt.Errorf("event definition %q source is required", definition.Key)
		}
		if definition.PayloadVersion <= 0 {
			return nil, fmt.Errorf("event definition %q payload version must be > 0", definition.Key)
		}
		if _, exists := items[definition.Key]; exists {
			return nil, fmt.Errorf("duplicate event definition %q", definition.Key)
		}
		definition.TemplatePaths = slices.Clone(definition.TemplatePaths)
		definition.Fields = slices.Clone(definition.Fields)
		items[definition.Key] = definition
	}
	return &Registry{definitions: items}, nil
}

func MustRegistry(definitions ...Definition) *Registry {
	registry, err := NewRegistry(definitions...)
	if err != nil {
		panic(err)
	}
	return registry
}

func (r *Registry) Definitions() []Definition {
	if r == nil {
		return nil
	}
	keys := make([]string, 0, len(r.definitions))
	for key := range r.definitions {
		keys = append(keys, string(key))
	}
	sort.Strings(keys)

	out := make([]Definition, 0, len(keys))
	for _, key := range keys {
		definition := r.definitions[Key(key)]
		definition.TemplatePaths = slices.Clone(definition.TemplatePaths)
		definition.Fields = slices.Clone(definition.Fields)
		out = append(out, definition)
	}
	return out
}

func (r *Registry) EventTypes() []string {
	definitions := r.Definitions()
	out := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, string(definition.Key))
	}
	return out
}

func (r *Registry) Get(key Key) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}
	definition, ok := r.definitions[key]
	if !ok {
		return Definition{}, false
	}
	definition.TemplatePaths = slices.Clone(definition.TemplatePaths)
	definition.Fields = slices.Clone(definition.Fields)
	return definition, true
}

func (r *Registry) IsKnown(key string) bool {
	_, ok := r.Get(Key(key))
	return ok
}
