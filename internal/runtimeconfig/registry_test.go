package runtimeconfig

import (
	"strings"
	"testing"

	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
)

func TestRendererProfileRegistryResolvesOneProfilePerEvent(t *testing.T) {
	registry := NewRendererProfileRegistry(map[string]domain.RendererProfile{
		"watcher-failed": testSlackProfile(domain.SourceWatcher, "watcher.deployment.failed"),
		"github-failed":  testSlackProfile(domain.SourceGitHub, "github.workflow.failed"),
	})
	selected := []string{"watcher-failed", "github-failed"}

	_, profileID, err := registry.Resolve(selected, domain.Event{EventEnvelope: domain.EventEnvelope{Source: domain.SourceGitHub, Key: "github.workflow.failed"}})
	if err != nil {
		t.Fatalf("resolve profile: %v", err)
	}
	if profileID != "github-failed" {
		t.Fatalf("resolved profile %q, want github-failed", profileID)
	}

	_, profileID, err = registry.Resolve(selected, domain.Event{EventEnvelope: domain.EventEnvelope{Source: domain.SourceWatcher, Key: "watcher.webhook.test"}})
	if err != nil {
		t.Fatalf("resolve unmatched event: %v", err)
	}
	if profileID != "" {
		t.Fatalf("resolved profile %q for unmatched event", profileID)
	}
}

func TestRendererProfileRegistryRejectsUnknownAndAmbiguousSelections(t *testing.T) {
	registry := NewRendererProfileRegistry(map[string]domain.RendererProfile{
		"failed-a": testSlackProfile(domain.SourceWatcher, "watcher.deployment.failed"),
		"failed-b": testSlackProfile(domain.SourceWatcher, "watcher.deployment.failed"),
	})
	event := domain.Event{EventEnvelope: domain.EventEnvelope{Source: domain.SourceWatcher, Key: "watcher.deployment.failed"}}

	if _, _, err := registry.Resolve([]string{"missing"}, event); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected missing profile error, got %v", err)
	}
	if _, _, err := registry.Resolve([]string{"failed-a", "failed-b"}, event); err == nil || !strings.Contains(err.Error(), "both match") {
		t.Fatalf("expected ambiguous profile error, got %v", err)
	}
}

func TestDestinationRegistryClonesRendererProfileSlices(t *testing.T) {
	profiles := []string{"watcher-failed"}
	registry := NewDestinationRegistry(map[string]config.DestinationConfig{
		"slack": {Type: domain.DestinationSlack, RendererProfiles: profiles},
	})
	profiles[0] = "mutated"

	got, ok := registry.Get("slack")
	if !ok || got.RendererProfiles[0] != "watcher-failed" {
		t.Fatalf("registry retained caller slice: %+v", got.RendererProfiles)
	}
	got.RendererProfiles[0] = "mutated-again"
	got, _ = registry.Get("slack")
	if got.RendererProfiles[0] != "watcher-failed" {
		t.Fatalf("registry exposed internal slice: %+v", got.RendererProfiles)
	}
}

func testSlackProfile(source domain.Source, key string) domain.RendererProfile {
	return domain.RendererProfile{
		Source: source,
		Key:    key,
		Templates: domain.RendererDestinationTemplates{
			Slack: &domain.SlackTemplate{Title: "{{.Title}}", Body: "{{.Summary}}"},
		},
	}
}
