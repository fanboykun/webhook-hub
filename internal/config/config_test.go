package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iweka-dev/webhook-hub/internal/domain"
)

func TestDefaultPathPrefersLocalConfig(t *testing.T) {
	t.Setenv("GATEWAY_CONFIG", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	dir := t.TempDir()
	localConfig := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(localConfig, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}
	exampleDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(exampleDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	exampleConfig := filepath.Join(exampleDir, "config.example.yaml")
	if err := os.WriteFile(exampleConfig, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatalf("write example config: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	if got := DefaultPath(); got != "config.yaml" {
		t.Fatalf("DefaultPath() = %q, want %q", got, "config.yaml")
	}
}

func TestDefaultPathFallsBackToExample(t *testing.T) {
	t.Setenv("GATEWAY_CONFIG", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	dir := t.TempDir()
	exampleDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(exampleDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	exampleConfig := filepath.Join(exampleDir, "config.example.yaml")
	if err := os.WriteFile(exampleConfig, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatalf("write example config: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	if got := DefaultPath(); got != "configs/config.example.yaml" {
		t.Fatalf("DefaultPath() = %q, want %q", got, "configs/config.example.yaml")
	}
}

func TestDefaultPathReturnsLocalNameWhenNoConfigExists(t *testing.T) {
	t.Setenv("GATEWAY_CONFIG", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	if got := DefaultPath(); got != "config.yaml" {
		t.Fatalf("DefaultPath() = %q, want %q", got, "config.yaml")
	}
}

func TestResolveSecretsRequiresWatcherWhsecPrefix(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "not-prefixed")

	cfg := Config{
		Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
		Database: DatabaseConfig{Path: "gateway.db"},
		Workers:  WorkersConfig{BatchSize: 1, Concurrency: 1},
		Retry:    RetryConfig{MaxAttempts: 1},
		Integrations: map[string]IntegrationConfig{
			"watcher-production": {
				Source:    domain.SourceWatcher,
				SecretEnv: "WATCHER_WEBHOOK_SECRET",
			},
		},
	}

	if err := cfg.resolveSecrets(); err == nil {
		t.Fatal("expected watcher secret format validation error")
	}
}

func TestResolveSecretsAcceptsWatcherWhsecPrefix(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "whsec_c2VjcmV0")

	cfg := Config{
		Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
		Database: DatabaseConfig{Path: "gateway.db"},
		Workers:  WorkersConfig{BatchSize: 1, Concurrency: 1},
		Retry:    RetryConfig{MaxAttempts: 1},
		Integrations: map[string]IntegrationConfig{
			"watcher-production": {
				Source:    domain.SourceWatcher,
				SecretEnv: "WATCHER_WEBHOOK_SECRET",
			},
		},
	}

	if err := cfg.resolveSecrets(); err != nil {
		t.Fatalf("resolveSecrets returned error: %v", err)
	}
	if got := cfg.Integrations["watcher-production"].ResolvedSecret; got != "whsec_c2VjcmV0" {
		t.Fatalf("ResolvedSecret = %q, want %q", got, "whsec_c2VjcmV0")
	}
}

func TestValidateAcceptsTelegramGroupChatID(t *testing.T) {
	cfg := Config{
		Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
		Database: DatabaseConfig{Path: "gateway.db"},
		Workers:  WorkersConfig{BatchSize: 1, Concurrency: 1},
		Retry:    RetryConfig{MaxAttempts: 1},
		Destinations: map[string]DestinationConfig{
			"telegram-group": {
				Type:        domain.DestinationTelegram,
				BotTokenEnv: "TELEGRAM_BOT_TOKEN",
				ChatID:      "-100123456789",
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid telegram group chat id, got %v", err)
	}
}

func TestValidateRejectsInvalidTelegramChatID(t *testing.T) {
	cfg := Config{
		Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
		Database: DatabaseConfig{Path: "gateway.db"},
		Workers:  WorkersConfig{BatchSize: 1, Concurrency: 1},
		Retry:    RetryConfig{MaxAttempts: 1},
		Destinations: map[string]DestinationConfig{
			"telegram-group": {
				Type:        domain.DestinationTelegram,
				BotTokenEnv: "TELEGRAM_BOT_TOKEN",
				ChatID:      "+1vxbHyRyEyRiN2M1",
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid telegram chat id error")
	}
}
