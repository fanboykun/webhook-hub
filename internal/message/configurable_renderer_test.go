package message

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/config"
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

func TestConfigurableRenderer_Resolution(t *testing.T) {
	profiles := map[string]config.ProfileConfig{
		"my-profile": {
			"watcher": config.SourceConfig{
				Default: config.DestinationTemplates{
					Slack: &config.SlackTemplateConfig{
						Title: "Default Title",
						Body:  "Default Body",
					},
				},
				Overrides: map[string]config.DestinationTemplates{
					"watcher.deployment.failed": {
						Slack: &config.SlackTemplateConfig{
							Title: "Failed Override Title",
							Body:  "Failed Override Body",
						},
					},
				},
			},
		},
	}

	r := NewConfigurableRenderer(runtimeconfig.NewRendererProfileRegistry(profiles), nil, nil)
	ctx := context.Background()

	// 1. Destination with no profile -> fall back
	destNoProfile := domain.Destination{ID: "d1", Type: domain.DestinationSlack, Profile: ""}
	evt := domain.Event{Source: domain.SourceWatcher, Type: "watcher.deployment.failed"}
	msg, err := r.Render(ctx, evt, destNoProfile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(string(msg.Body), "profile=") {
		t.Errorf("expected fallback slack renderer, got: %s", string(msg.Body))
	}

	// 2. Profile doesn't exist -> fall back
	destBadProfile := domain.Destination{ID: "d2", Type: domain.DestinationSlack, Profile: "non-existent"}
	msg, err = r.Render(ctx, evt, destBadProfile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(string(msg.Body), "profile=") {
		t.Errorf("expected fallback slack renderer, got: %s", string(msg.Body))
	}

	// 3. Source doesn't exist in profile -> fall back
	destGoodProfile := domain.Destination{ID: "d3", Type: domain.DestinationSlack, Profile: "my-profile"}
	evtBadSource := domain.Event{Source: domain.SourceGitHub, Type: "github.pull_request.opened"}
	msg, err = r.Render(ctx, evtBadSource, destGoodProfile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(string(msg.Body), "profile=my-profile") {
		t.Errorf("expected fallback slack renderer, got: %s", string(msg.Body))
	}

	// 4. Matches default template (e.g. event type has no override)
	evtDefault := domain.Event{Source: domain.SourceWatcher, Type: "watcher.deployment.started"}
	msg, err = r.Render(ctx, evtDefault, destGoodProfile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(string(msg.Body), "Default Title") {
		t.Errorf("expected Default Title, got: %s", string(msg.Body))
	}

	// 5. Matches override template
	evtOverride := domain.Event{Source: domain.SourceWatcher, Type: "watcher.deployment.failed"}
	msg, err = r.Render(ctx, evtOverride, destGoodProfile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(string(msg.Body), "Failed Override Title") {
		t.Errorf("expected Failed Override Title, got: %s", string(msg.Body))
	}
}

func TestGoldenIntegration(t *testing.T) {
	profiles := map[string]config.ProfileConfig{
		"templated-profile": {
			"watcher": config.SourceConfig{
				Default: config.DestinationTemplates{
					Slack: &config.SlackTemplateConfig{
						Title: "[{{.Severity}}] {{.Title}}",
						Body:  "Service {{.Service}} environment {{.Environment}}",
					},
					Telegram: &config.TelegramTemplateConfig{
						Text: "<b>[{{.Severity}}] {{.Title}}</b>\nService: {{.Service}}",
					},
				},
				Overrides: map[string]config.DestinationTemplates{
					"watcher.deployment.failed": {
						Slack: &config.SlackTemplateConfig{
							Title: "CRITICAL ALERT: {{.Title}} failed in {{.Environment}}",
							Body:  "*Service:* {{.Service}}\n*Release:* {{.Release}}\n*Error/Summary:* {{.Summary}}",
						},
						Telegram: &config.TelegramTemplateConfig{
							Text: "🚨 <b>{{.Title}} ({{.Lifecycle}})</b> 🚨\nEnvironment: <b>{{.Environment}}</b>\nService: <code>{{.Service}}</code>\nRelease: <code>{{.Release}}</code>\nSummary: <i>{{.Summary}}</i>\n<a href=\"{{.URL}}\">View Details</a>",
						},
					},
				},
			},
		},
	}

	r := NewConfigurableRenderer(runtimeconfig.NewRendererProfileRegistry(profiles), nil, nil)
	ctx := context.Background()

	evt := domain.Event{
		ID:          "evt_1",
		Source:      domain.SourceWatcher,
		Type:        "watcher.deployment.failed",
		Severity:    domain.SeverityError,
		Lifecycle:   domain.LifecycleFailed,
		Title:       "Deployment failed",
		Summary:     "Deployment of api-prod to v1.4.3 failed during health_check: health check returned 503 <error>",
		Service:     "api-prod",
		Environment: "production",
		Release:     "v1.4.3",
		CommitSHA:   "abc12345",
		Actor:       "agent",
		URL:         "https://watcher.example.com/attempts/302",
		OccurredAt:  time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		name        string
		destination domain.Destination
		goldenFile  string
	}{
		{
			name:        "Slack Default (Fallback)",
			destination: domain.Destination{ID: "slack_fallback", Type: domain.DestinationSlack, Profile: ""},
			goldenFile:  "slack_fallback.json",
		},
		{
			name:        "Slack Templated Override",
			destination: domain.Destination{ID: "slack_templated", Type: domain.DestinationSlack, Profile: "templated-profile"},
			goldenFile:  "slack_templated.json",
		},
		{
			name:        "Telegram Default (Fallback)",
			destination: domain.Destination{ID: "telegram_fallback", Type: domain.DestinationTelegram, Profile: ""},
			goldenFile:  "telegram_fallback.html",
		},
		{
			name:        "Telegram Templated Override",
			destination: domain.Destination{ID: "telegram_templated", Type: domain.DestinationTelegram, Profile: "templated-profile"},
			goldenFile:  "telegram_templated.html",
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
