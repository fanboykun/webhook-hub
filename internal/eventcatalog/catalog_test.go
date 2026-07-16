package eventcatalog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fanboykun/webhook-hub/internal/domain"
)

func TestNormalizeEnvelopeEnforcesAndMaterializesPayloadContract(t *testing.T) {
	registry := MustRegistry(testDefinition())
	envelope := domain.EventEnvelope{
		Source:         domain.SourceWatcher,
		Key:            "watcher.test",
		PayloadVersion: 1,
		PayloadJSON:    []byte(`{"required":{"name":"api"}}`),
	}

	if err := registry.NormalizeEnvelope(&envelope); err != nil {
		t.Fatalf("normalize envelope: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(envelope.PayloadJSON, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if payload["optional"] != "" {
		t.Fatalf("expected optional scalar to be materialized, got %#v", payload["optional"])
	}
	required := payload["required"].(map[string]any)
	if required["count"] != float64(0) {
		t.Fatalf("expected optional nested number to be materialized, got %#v", required["count"])
	}
}

func TestNormalizeEnvelopeRejectsContractViolations(t *testing.T) {
	registry := MustRegistry(testDefinition())
	tests := []struct {
		name    string
		version int
		payload string
		want    string
	}{
		{name: "wrong version", version: 2, payload: `{"required":{"name":"api"}}`, want: "does not match contract version"},
		{name: "missing required object", version: 1, payload: `{}`, want: `required field "required" is missing`},
		{name: "missing required nested field", version: 1, payload: `{"required":{}}`, want: `required field "required.name" is missing`},
		{name: "wrong scalar type", version: 1, payload: `{"required":{"name":42}}`, want: `field "required.name" must be string`},
		{name: "unknown field", version: 1, payload: `{"required":{"name":"api","extra":true}}`, want: `field "required.extra" is not declared`},
		{name: "trailing json", version: 1, payload: `{"required":{"name":"api"}} {}`, want: "one JSON value"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			envelope := domain.EventEnvelope{Source: domain.SourceWatcher, Key: "watcher.test", PayloadVersion: tc.version, PayloadJSON: []byte(tc.payload)}
			err := registry.NormalizeEnvelope(&envelope)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NormalizeEnvelope() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestNormalizeEnvelopeRejectsUnsafeSourceURL(t *testing.T) {
	registry := MustRegistry(testDefinition())
	envelope := domain.EventEnvelope{
		Source:         domain.SourceWatcher,
		Key:            "watcher.test",
		PayloadVersion: 1,
		PayloadJSON:    []byte(`{"required":{"name":"api"}}`),
		SourceURL:      "javascript:alert(1)",
	}

	err := registry.NormalizeEnvelope(&envelope)
	if err == nil || !strings.Contains(err.Error(), "absolute HTTP(S) URL") {
		t.Fatalf("expected unsafe source URL error, got %v", err)
	}
}

func TestNewRegistryRejectsInvalidFieldSchemas(t *testing.T) {
	tests := []struct {
		name   string
		fields []Field
	}{
		{name: "invalid path", fields: []Field{{Path: "Bad", Type: FieldTypeString}}},
		{name: "unknown type", fields: []Field{{Path: "value", Type: "integer"}}},
		{name: "duplicate path", fields: []Field{{Path: "value", Type: FieldTypeString}, {Path: "value", Type: FieldTypeString}}},
		{name: "missing object parent", fields: []Field{{Path: "parent.child", Type: FieldTypeString}}},
		{name: "scalar parent", fields: []Field{{Path: "parent", Type: FieldTypeString}, {Path: "parent.child", Type: FieldTypeString}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRegistry(Definition{Key: "watcher.test", Source: domain.SourceWatcher, PayloadVersion: 1, Fields: tc.fields})
			if err == nil {
				t.Fatal("expected invalid field schema error")
			}
		})
	}
}

func testDefinition() Definition {
	return Definition{
		Key:            "watcher.test",
		Source:         domain.SourceWatcher,
		PayloadVersion: 1,
		Fields: []Field{
			{Path: "required", Type: FieldTypeObject, Required: true},
			{Path: "required.name", Type: FieldTypeString, Required: true},
			{Path: "required.count", Type: FieldTypeNumber},
			{Path: "optional", Type: FieldTypeString},
		},
	}
}
