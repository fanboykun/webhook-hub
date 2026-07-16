package renderprofile

import (
	"strings"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/eventcatalog"
	watchercatalog "github.com/fanboykun/webhook-hub/internal/eventcatalog/watcher"
)

func TestCompileTemplateAcceptsTypedScalarFields(t *testing.T) {
	definition := watcherDefinition(t, watchercatalog.EventDeploymentFailed)
	compiled, err := CompileTemplate("valid", `{{.Title}} {{.Payload.attempt.target_version}} {{.OccurredAt.Format "2006-01-02"}}`, definition)
	if err != nil {
		t.Fatalf("compile valid template: %v", err)
	}
	got, err := Execute(compiled, Context{
		Title:      "failed",
		OccurredAt: time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"attempt": map[string]any{"target_version": "v2"},
		},
	})
	if err != nil {
		t.Fatalf("execute template: %v", err)
	}
	if got != "failed v2 2026-07-16" {
		t.Fatalf("unexpected output %q", got)
	}
}

func TestCompileTemplateRejectsUntypedOrDynamicConstructs(t *testing.T) {
	definition := watcherDefinition(t, watchercatalog.EventDeploymentFailed)
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "whole context", source: `{{.}}`, want: "whole-context"},
		{name: "whole payload", source: `{{.Payload}}`, want: "whole-payload"},
		{name: "payload object", source: `{{.Payload.attempt}}`, want: "cannot be rendered directly"},
		{name: "unknown payload field", source: `{{.Payload.attempt.nope}}`, want: "not declared"},
		{name: "cross event field", source: `{{.Payload.health.current_status}}`, want: "not declared"},
		{name: "metadata", source: `{{.Metadata}}`, want: "metadata is not part"},
		{name: "function", source: `{{printf "%s" .Title}}`, want: "function"},
		{name: "range", source: `{{range .Payload}}{{.}}{{end}}`, want: "ranges are not allowed"},
		{name: "with", source: `{{with .Payload.attempt}}{{.}}{{end}}`, want: "with blocks are not allowed"},
		{name: "variable", source: `{{$title := .Title}}{{$title}}`, want: "variables are not allowed"},
		{name: "associated template", source: `{{define "inner"}}{{.Title}}{{end}}{{template "inner" .}}`, want: "associated template"},
		{name: "invalid chain", source: `{{.Title.String}}`, want: "field chain"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CompileTemplate("invalid", tc.source, definition)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CompileTemplate() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestCompileTemplateRejectsFieldsFromAnotherWatcherEvent(t *testing.T) {
	definition := watcherDefinition(t, watchercatalog.EventWebhookTest)
	_, err := CompileTemplate("isolated", `{{.Payload.attempt.target_version}}`, definition)
	if err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("expected event-isolated payload error, got %v", err)
	}
}

func TestCompileTemplateRejectsOversizedSource(t *testing.T) {
	definition := watcherDefinition(t, watchercatalog.EventWebhookTest)
	_, err := CompileTemplate("large", strings.Repeat("x", MaxTemplateBytes+1), definition)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size limit error, got %v", err)
	}
}

func watcherDefinition(t *testing.T, key watchercatalog.Key) eventcatalog.Definition {
	t.Helper()
	for _, definition := range watchercatalog.Definitions() {
		if definition.Key == eventcatalog.Key(key) {
			return definition
		}
	}
	t.Fatalf("watcher definition %q not found", key)
	return eventcatalog.Definition{}
}
