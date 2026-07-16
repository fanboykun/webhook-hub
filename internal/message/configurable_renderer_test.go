package message

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
)

var update = flag.Bool("update", false, "update golden files")

func TestEscapeSlack(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello & welcome", "hello &amp; welcome"},
		{"<alert>", "&lt;alert&gt;"},
		{"normal text", "normal text"},
	}

	for _, tc := range tests {
		got := escapeSlack(tc.input)
		if got != tc.expected {
			t.Errorf("escapeSlack(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestEscapeTelegram(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello & welcome", "hello &amp; welcome"},
		{"<alert>", "&lt;alert&gt;"},
		{"quotes \"test\"", "quotes &quot;test&quot;"},
		{"normal text", "normal text"},
	}

	for _, tc := range tests {
		got := escapeTelegram(tc.input)
		if got != tc.expected {
			t.Errorf("escapeTelegram(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestEscapeTeamsMarkdown(t *testing.T) {
	got := escapeTeamsMarkdown(`status & [check](https://example.invalid/a_(b)) <error>`)
	want := `status &amp; \[check\]\(https://example.invalid/a\_\(b\)\) &lt;error&gt;`
	if got != want {
		t.Fatalf("escapeTeamsMarkdown() = %q, want %q", got, want)
	}
}

func TestConfigurableRenderer_Resolution(t *testing.T) {
	profiles := map[string]domain.RendererProfile{
		"my-profile": {
			Source: domain.SourceWatcher,
			Key:    "watcher.deployment.failed",
			Templates: domain.RendererDestinationTemplates{
				Slack: &domain.SlackTemplate{
					Title: "Failed Override Title",
					Body:  "Failed Override Body",
				},
			},
		},
	}

	r := NewConfigurableRenderer(runtimeconfig.NewRendererProfileRegistry(profiles), nil, nil)
	ctx := context.Background()

	// 1. Destination with no profile -> fall back
	destNoProfile := domain.Destination{ID: "d1", Type: domain.DestinationSlack}
	evt := domain.Event{EventEnvelope: domain.EventEnvelope{Source: domain.SourceWatcher, Key: "watcher.deployment.failed"}}
	msg, err := r.Render(ctx, evt, destNoProfile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(string(msg.Body), "profile=") {
		t.Errorf("expected fallback slack renderer, got: %s", string(msg.Body))
	}

	// 2. An unknown profile is a broken destination contract.
	destBadProfile := domain.Destination{ID: "d2", Type: domain.DestinationSlack, RendererProfiles: []string{"non-existent"}}
	_, err = r.Render(ctx, evt, destBadProfile)
	if err == nil {
		t.Fatal("expected an unknown renderer profile error")
	}

	// 3. Event doesn't exist in profile -> fall back
	destGoodProfile := domain.Destination{ID: "d3", Type: domain.DestinationSlack, RendererProfiles: []string{"my-profile"}}
	evtNoBinding := domain.Event{EventEnvelope: domain.EventEnvelope{Source: domain.SourceGitHub, Key: "github.pull_request.opened"}}
	msg, err = r.Render(ctx, evtNoBinding, destGoodProfile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(string(msg.Body), "profile=") || contains(string(msg.Body), "profile=my-profile") {
		t.Errorf("expected fallback slack renderer, got: %s", string(msg.Body))
	}

	// 4. Same source but different event -> fall back because one profile belongs to one event.
	evtDefault := domain.Event{EventEnvelope: domain.EventEnvelope{Source: domain.SourceWatcher, Key: "watcher.deployment.started"}}
	msg, err = r.Render(ctx, evtDefault, destGoodProfile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contains(string(msg.Body), "Failed Override Title") {
		t.Errorf("expected fallback for non-matching event, got: %s", string(msg.Body))
	}

	// 5. Matches override template
	evtOverride := domain.Event{EventEnvelope: domain.EventEnvelope{
		Source: domain.SourceWatcher, Key: "watcher.deployment.failed", PayloadVersion: 1,
		PayloadJSON: []byte(`{"watcher":{},"service":{},"attempt":{}}`),
	}}
	msg, err = r.Render(ctx, evtOverride, destGoodProfile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(string(msg.Body), "Failed Override Title") {
		t.Errorf("expected Failed Override Title, got: %s", string(msg.Body))
	}
}

func TestGoldenIntegration(t *testing.T) {
	profiles := map[string]domain.RendererProfile{
		"templated-profile": {
			Source: domain.SourceWatcher,
			Key:    "watcher.deployment.failed",
			Templates: domain.RendererDestinationTemplates{
				Slack: &domain.SlackTemplate{
					Title: "CRITICAL ALERT: {{.Title}} failed in {{.Environment}}",
					Body:  "*Service:* {{.Payload.service.name}}\n*Release:* {{.Payload.attempt.target_version}}\n*Error/Summary:* {{.Summary}}",
				},
				Telegram: &domain.TelegramTemplate{
					Text: "🚨 <b>{{.Title}} ({{.Lifecycle}})</b> 🚨\nEnvironment: <b>{{.Environment}}</b>\nService: <code>{{.Payload.service.name}}</code>\nRelease: <code>{{.Payload.attempt.target_version}}</code>\nSummary: <i>{{.Summary}}</i>\n<a href=\"{{.SourceURL}}\">View Details</a>",
				},
				Teams: &domain.TeamsTemplate{
					Title: "CRITICAL ALERT: {{.Title}} failed in {{.Environment}}",
					Body:  "**Service:** {{.Payload.service.name}}\n\n**Release:** {{.Payload.attempt.target_version}}\n\n{{.Summary}}",
				},
			},
		},
	}

	r := NewConfigurableRenderer(runtimeconfig.NewRendererProfileRegistry(profiles), nil, nil)
	ctx := context.Background()

	evt := domain.Event{
		ID: "evt_1",
		EventEnvelope: domain.EventEnvelope{
			Source:         domain.SourceWatcher,
			Key:            "watcher.deployment.failed",
			Severity:       domain.SeverityError,
			Lifecycle:      domain.LifecycleFailed,
			Title:          "Deployment failed",
			Summary:        "Deployment of api-prod to v1.4.3 failed during health_check: health check returned 503 <error>",
			Scope:          domain.EventScope{Service: "api-prod", Environment: "production"},
			SourceURL:      "https://watcher.example.com/attempts/302",
			OccurredAt:     time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC),
			PayloadVersion: 1,
			MetadataJSON:   []byte(`{"release":"v1.4.3","commit_sha":"abc12345","actor":"agent"}`),
			PayloadJSON:    []byte(`{"summary":"Deployment of api-prod to v1.4.3 failed during health_check: health check returned 503 <error>","watcher":{"id":12,"name":"api-prod"},"service":{"id":87,"name":"api-prod","service_type":"nssm"},"attempt":{"id":302,"target_version":"v1.4.3","from_version":"v1.4.2","failure_phase":"health_check","error":"health check returned 503"}}`),
		},
	}

	tests := []struct {
		name        string
		destination domain.Destination
		goldenFile  string
	}{
		{
			name:        "Slack Default (Fallback)",
			destination: domain.Destination{ID: "slack_fallback", Type: domain.DestinationSlack},
			goldenFile:  "slack_fallback.json",
		},
		{
			name:        "Slack Templated Override",
			destination: domain.Destination{ID: "slack_templated", Type: domain.DestinationSlack, RendererProfiles: []string{"templated-profile"}},
			goldenFile:  "slack_templated.json",
		},
		{
			name:        "Telegram Default (Fallback)",
			destination: domain.Destination{ID: "telegram_fallback", Type: domain.DestinationTelegram},
			goldenFile:  "telegram_fallback.html",
		},
		{
			name:        "Telegram Templated Override",
			destination: domain.Destination{ID: "telegram_templated", Type: domain.DestinationTelegram, RendererProfiles: []string{"templated-profile"}},
			goldenFile:  "telegram_templated.html",
		},
		{
			name:        "Teams Default (Fallback)",
			destination: domain.Destination{ID: "teams_fallback", Type: domain.DestinationTeams},
			goldenFile:  "teams_fallback.json",
		},
		{
			name:        "Teams Templated Override",
			destination: domain.Destination{ID: "teams_templated", Type: domain.DestinationTeams, RendererProfiles: []string{"templated-profile"}},
			goldenFile:  "teams_templated.json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := r.Render(ctx, evt, tc.destination)
			if err != nil {
				t.Fatalf("failed to render: %v", err)
			}

			goldenPath := filepath.Join("testdata", tc.goldenFile)

			if *update {
				if err := os.MkdirAll("testdata", 0755); err != nil {
					t.Fatalf("failed to create testdata dir: %v", err)
				}
				if err := os.WriteFile(goldenPath, msg.Body, 0644); err != nil {
					t.Fatalf("failed to write golden file: %v", err)
				}
			}

			// Read golden file (if not exists and not updating, write it automatically to ease testing setup)
			goldenBytes, err := os.ReadFile(goldenPath)
			if os.IsNotExist(err) && !*update {
				if err := os.MkdirAll("testdata", 0755); err != nil {
					t.Fatalf("failed to create testdata dir: %v", err)
				}
				if err := os.WriteFile(goldenPath, msg.Body, 0644); err != nil {
					t.Fatalf("failed to write golden file: %v", err)
				}
				goldenBytes = msg.Body
			} else if err != nil {
				t.Fatalf("failed to read golden file: %v", err)
			}

			if string(msg.Body) != string(goldenBytes) {
				t.Errorf("rendered output does not match golden file %s\nGot:\n%s\nWant:\n%s", tc.goldenFile, string(msg.Body), string(goldenBytes))
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (s != "" && strings.Index(s, substr) >= 0))
}
