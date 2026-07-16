package eventcatalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/fanboykun/webhook-hub/internal/domain"
)

type Key string

type Field struct {
	Path        string
	Type        string
	Required    bool
	Description string
}

const (
	FieldTypeString  = "string"
	FieldTypeNumber  = "number"
	FieldTypeBoolean = "boolean"
	FieldTypeObject  = "object"
)

type Definition struct {
	Key            Key
	Source         domain.Source
	PayloadVersion int
	Summary        string
	Fields         []Field
}

type Registry struct {
	definitions map[Key]Definition
}

var fieldPathPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)

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
		if err := validateFields(definition); err != nil {
			return nil, err
		}
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
	definition.Fields = slices.Clone(definition.Fields)
	return definition, true
}

func (r *Registry) NormalizeEnvelope(envelope *domain.EventEnvelope) error {
	if envelope == nil {
		return fmt.Errorf("event envelope is required")
	}
	definition, ok := r.Resolve(envelope.Source, envelope.Key)
	if !ok {
		return fmt.Errorf("unknown event contract %s:%s", envelope.Source, envelope.Key)
	}
	if envelope.PayloadVersion != definition.PayloadVersion {
		return fmt.Errorf("event %s:%s payload version %d does not match contract version %d", envelope.Source, envelope.Key, envelope.PayloadVersion, definition.PayloadVersion)
	}
	if err := validateSourceURL(envelope.SourceURL); err != nil {
		return fmt.Errorf("event %s:%s source URL: %w", envelope.Source, envelope.Key, err)
	}

	payload, err := normalizePayload(definition, envelope.PayloadJSON)
	if err != nil {
		return fmt.Errorf("event %s:%s payload: %w", envelope.Source, envelope.Key, err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode normalized payload: %w", err)
	}
	envelope.PayloadJSON = encoded
	return nil
}

func validateSourceURL(value string) error {
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("must be an absolute HTTP(S) URL")
	}
	return nil
}

func (r *Registry) MaterializePayload(source domain.Source, key string, payloadVersion int, raw []byte) (map[string]any, error) {
	definition, ok := r.Resolve(source, key)
	if !ok {
		return nil, fmt.Errorf("unknown event contract %s:%s", source, key)
	}
	if payloadVersion != definition.PayloadVersion {
		return nil, fmt.Errorf("payload version %d does not match contract version %d", payloadVersion, definition.PayloadVersion)
	}
	return normalizePayload(definition, raw)
}

func validateFields(definition Definition) error {
	seen := make(map[string]Field, len(definition.Fields))
	for _, field := range definition.Fields {
		if !fieldPathPattern.MatchString(field.Path) {
			return fmt.Errorf("event definition %q has invalid field path %q", definition.Key, field.Path)
		}
		switch field.Type {
		case FieldTypeString, FieldTypeNumber, FieldTypeBoolean, FieldTypeObject:
		default:
			return fmt.Errorf("event definition %q field %q has unsupported type %q", definition.Key, field.Path, field.Type)
		}
		if _, exists := seen[field.Path]; exists {
			return fmt.Errorf("event definition %q has duplicate field %q", definition.Key, field.Path)
		}
		seen[field.Path] = field
	}
	for path := range seen {
		parent, ok := parentPath(path)
		if !ok {
			continue
		}
		parentField, exists := seen[parent]
		if !exists || parentField.Type != FieldTypeObject {
			return fmt.Errorf("event definition %q field %q requires object parent %q", definition.Key, path, parent)
		}
	}
	return nil
}

func normalizePayload(definition Definition, raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("payload is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("payload must be a JSON object")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}

	declared := make(map[string]Field, len(definition.Fields))
	for _, field := range definition.Fields {
		declared[field.Path] = field
		value, exists := lookupPath(payload, field.Path)
		if !exists {
			if field.Required {
				return nil, fmt.Errorf("required field %q is missing", field.Path)
			}
			continue
		}
		if !matchesType(value, field.Type) {
			return nil, fmt.Errorf("field %q must be %s", field.Path, field.Type)
		}
	}

	for _, path := range payloadPaths(payload, "") {
		if _, ok := declared[path]; !ok {
			return nil, fmt.Errorf("field %q is not declared by the event contract", path)
		}
	}

	for _, field := range definition.Fields {
		if _, exists := lookupPath(payload, field.Path); !exists {
			setPath(payload, field.Path, zeroValue(field.Type))
		}
	}
	return payload, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("payload must contain one JSON value")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func lookupPath(payload map[string]any, path string) (any, bool) {
	var current any = payload
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func setPath(payload map[string]any, path string, value any) {
	segments := strings.Split(path, ".")
	current := payload
	for _, segment := range segments[:len(segments)-1] {
		next, ok := current[segment].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[segment] = next
		}
		current = next
	}
	current[segments[len(segments)-1]] = value
}

func payloadPaths(value any, prefix string) []string {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	paths := make([]string, 0, len(object))
	for key, child := range object {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		paths = append(paths, path)
		paths = append(paths, payloadPaths(child, path)...)
	}
	return paths
}

func matchesType(value any, fieldType string) bool {
	switch fieldType {
	case FieldTypeString:
		_, ok := value.(string)
		return ok
	case FieldTypeNumber:
		_, ok := value.(json.Number)
		return ok
	case FieldTypeBoolean:
		_, ok := value.(bool)
		return ok
	case FieldTypeObject:
		_, ok := value.(map[string]any)
		return ok
	default:
		return false
	}
}

func zeroValue(fieldType string) any {
	switch fieldType {
	case FieldTypeString:
		return ""
	case FieldTypeNumber:
		return json.Number("0")
	case FieldTypeBoolean:
		return false
	case FieldTypeObject:
		return map[string]any{}
	default:
		return nil
	}
}

func parentPath(path string) (string, bool) {
	index := strings.LastIndexByte(path, '.')
	if index < 0 {
		return "", false
	}
	return path[:index], true
}

func (r *Registry) Resolve(source domain.Source, key string) (Definition, bool) {
	definition, ok := r.Get(Key(key))
	if !ok {
		return Definition{}, false
	}
	if definition.Source != source {
		return Definition{}, false
	}
	return definition, true
}

func (r *Registry) IsKnown(key string) bool {
	_, ok := r.Get(Key(key))
	return ok
}
