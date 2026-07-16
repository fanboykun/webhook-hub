package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fanboykun/webhook-hub/internal/domain"
)

const testEncryptionKeyEnv = "GATEWAY_ENCRYPTION_KEY"
const testEncryptionKeyValue = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func testDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Path:             "gateway.db",
		EncryptionKeyEnv: testEncryptionKeyEnv,
	}
}

func testWorkersConfig() WorkersConfig {
	return WorkersConfig{
		BatchSize:        1,
		Concurrency:      1,
		PollInterval:     time.Second,
		LeaseDuration:    time.Minute,
		RecoveryInterval: time.Minute,
	}
}

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

func TestLoadExampleConfig(t *testing.T) {
	t.Setenv("GATEWAY_ADMIN_TOKEN", "admin-token")
	t.Setenv(testEncryptionKeyEnv, testEncryptionKeyValue)
	t.Setenv("WATCHER_WEBHOOK_SECRET", "whsec_c2VjcmV0")
	t.Setenv("GITHUB_WEBHOOK_SECRET", "github-secret")
	t.Setenv("SLACK_DEPLOYMENTS_WEBHOOK_URL", "https://example.invalid/slack")
	t.Setenv("TELEGRAM_ONCALL_BOT_TOKEN", "telegram-token")
	t.Setenv("TEAMS_ONCALL_WEBHOOK_URL", "https://example.invalid/teams")

	cfg, err := Load(filepath.Join("..", "..", "configs", "config.example.yaml"))
	if err != nil {
		t.Fatalf("load example config: %v", err)
	}
	if got := cfg.Destinations["slack-deployments"].RendererProfiles; len(got) != 2 {
		t.Fatalf("expected two event-specific Slack profiles, got %+v", got)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("expected two default routes, got %+v", cfg.Routes)
	}
}

func TestResolveSecretsRequiresWatcherWhsecPrefix(t *testing.T) {
	t.Setenv("WATCHER_WEBHOOK_SECRET", "not-prefixed")
	t.Setenv(testEncryptionKeyEnv, testEncryptionKeyValue)

	cfg := Config{
		Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
		Database: testDatabaseConfig(),
		Workers:  testWorkersConfig(),
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
	t.Setenv(testEncryptionKeyEnv, testEncryptionKeyValue)

	cfg := Config{
		Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
		Database: testDatabaseConfig(),
		Workers:  testWorkersConfig(),
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

func TestResolveSecretValueRejectsInlineSecrets(t *testing.T) {
	for _, value := range []string{"https://hooks.example.invalid/secret", "whsec_inline", "123456:token"} {
		if _, _, err := resolveSecretValue(value); err == nil || !strings.Contains(err.Error(), "environment variable name") {
			t.Fatalf("resolveSecretValue(%q) error = %v, want env reference error", value, err)
		}
	}
}

func TestValidateAcceptsTelegramGroupChatID(t *testing.T) {
	cfg := Config{
		Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
		Database: testDatabaseConfig(),
		Workers:  testWorkersConfig(),
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

func TestValidateAcceptsTeamsWebhookDestination(t *testing.T) {
	cfg := Config{
		Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
		Database: testDatabaseConfig(),
		Workers:  testWorkersConfig(),
		Retry:    RetryConfig{MaxAttempts: 1},
		Destinations: map[string]DestinationConfig{
			"teams-oncall": {
				Type:          domain.DestinationTeams,
				WebhookURLEnv: "TEAMS_ONCALL_WEBHOOK_URL",
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid teams webhook destination, got %v", err)
	}
}

func TestValidateRejectsWhitespaceResourceIDs(t *testing.T) {
	cfg := Config{
		Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
		Database: testDatabaseConfig(),
		Workers:  testWorkersConfig(),
		Retry:    RetryConfig{MaxAttempts: 1},
		Integrations: map[string]IntegrationConfig{
			" watcher-production ": {Source: domain.SourceWatcher},
		},
		Destinations: map[string]DestinationConfig{
			" slack-dest ": {Type: domain.DestinationSlack, WebhookURLEnv: "SLACK_URL"},
		},
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "without surrounding whitespace") {
		t.Fatalf("expected resource id validation error, got %v", err)
	}
}

func TestValidateDefaultRoutes(t *testing.T) {
	base := func() Config {
		return Config{
			Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
			Database: testDatabaseConfig(),
			Workers:  testWorkersConfig(),
			Retry:    RetryConfig{MaxAttempts: 1},
			Destinations: map[string]DestinationConfig{
				"slack-dest": {Type: domain.DestinationSlack, WebhookURLEnv: "SLACK_URL"},
			},
		}
	}

	tests := []struct {
		name  string
		route RouteConfig
		want  string
	}{
		{
			name: "valid",
			route: RouteConfig{
				ID:           "watcher-failed",
				Match:        RouteMatchConfig{Sources: []domain.Source{domain.SourceWatcher}, Types: []string{"watcher.deployment.failed"}},
				Destinations: []string{"slack-dest"},
			},
		},
		{name: "unknown destination", route: RouteConfig{ID: "bad-destination", Destinations: []string{"missing"}}, want: "undefined destination"},
		{name: "unknown event", route: RouteConfig{ID: "bad-event", Match: RouteMatchConfig{Types: []string{"watcher.nope"}}, Destinations: []string{"slack-dest"}}, want: "unknown event type"},
		{name: "duplicate destination", route: RouteConfig{ID: "duplicate", Destinations: []string{"slack-dest", "slack-dest"}}, want: "repeats destination"},
		{name: "empty destinations", route: RouteConfig{ID: "empty"}, want: "must not be empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			cfg.Routes = []RouteConfig{tc.route}
			err := cfg.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestValidateRejectsInvalidTelegramChatID(t *testing.T) {
	cfg := Config{
		Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
		Database: testDatabaseConfig(),
		Workers:  testWorkersConfig(),
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

func TestValidateRendererProfiles(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config with profile",
			cfg: Config{
				Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
				Database: testDatabaseConfig(),
				Workers:  testWorkersConfig(),
				Retry:    RetryConfig{MaxAttempts: 1},
				Destinations: map[string]DestinationConfig{
					"slack-dest": {
						Type:             domain.DestinationSlack,
						WebhookURLEnv:    "SLACK_URL",
						RendererProfiles: []string{"watcher-deployment-failed-detailed"},
					},
				},
				RendererProfiles: map[string]ProfileConfig{
					"watcher-deployment-failed-detailed": {
						Source: domain.SourceWatcher,
						Key:    "watcher.deployment.failed",
						Templates: DestinationTemplates{
							Slack: &SlackTemplateConfig{
								Title: "[{{.Severity}}] {{.Title}}",
								Body:  "{{.Summary}} - {{.OccurredAt.Format \"2006-01-02\"}} {{.Payload.attempt.target_version}}",
							},
							Teams: &TeamsTemplateConfig{
								Title: "[{{.Severity}}] {{.Title}}",
								Body:  "{{.Payload.service.name}}",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid template syntax",
			cfg: Config{
				Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
				Database: testDatabaseConfig(),
				Workers:  testWorkersConfig(),
				Retry:    RetryConfig{MaxAttempts: 1},
				RendererProfiles: map[string]ProfileConfig{
					"watcher-deployment-failed-detailed": {
						Source: domain.SourceWatcher,
						Key:    "watcher.deployment.failed",
						Templates: DestinationTemplates{
							Slack: &SlackTemplateConfig{
								Title: "[{{.Severity}",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid template field",
			cfg: Config{
				Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
				Database: testDatabaseConfig(),
				Workers:  testWorkersConfig(),
				Retry:    RetryConfig{MaxAttempts: 1},
				RendererProfiles: map[string]ProfileConfig{
					"watcher-deployment-failed-detailed": {
						Source: domain.SourceWatcher,
						Key:    "watcher.deployment.failed",
						Templates: DestinationTemplates{
							Slack: &SlackTemplateConfig{
								Title: "[{{.ReceiptID}}]",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "undefined profile reference falls back to built-in renderer",
			cfg: Config{
				Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
				Database: testDatabaseConfig(),
				Workers:  testWorkersConfig(),
				Retry:    RetryConfig{MaxAttempts: 1},
				Destinations: map[string]DestinationConfig{
					"slack-dest": {
						Type:             domain.DestinationSlack,
						WebhookURLEnv:    "SLACK_URL",
						RendererProfiles: []string{"nonexistent"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "unknown event type is rejected",
			cfg: Config{
				Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
				Database: testDatabaseConfig(),
				Workers:  testWorkersConfig(),
				Retry:    RetryConfig{MaxAttempts: 1},
				RendererProfiles: map[string]ProfileConfig{
					"watcher-unknown-detailed": {
						Source: domain.SourceWatcher,
						Key:    "watcher.invalid.event",
						Templates: DestinationTemplates{
							Slack: &SlackTemplateConfig{
								Title: "{{.Title}}",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "metadata access is rejected because it is not typed payload",
			cfg: Config{
				Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
				Database: testDatabaseConfig(),
				Workers:  testWorkersConfig(),
				Retry:    RetryConfig{MaxAttempts: 1},
				RendererProfiles: map[string]ProfileConfig{
					"watcher-deployment-failed-detailed": {
						Source: domain.SourceWatcher,
						Key:    "watcher.deployment.failed",
						Templates: DestinationTemplates{
							Slack: &SlackTemplateConfig{
								Title: `{{ index .Metadata "release" }}`,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "index payload access is rejected because it bypasses typed paths",
			cfg: Config{
				Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
				Database: testDatabaseConfig(),
				Workers:  testWorkersConfig(),
				Retry:    RetryConfig{MaxAttempts: 1},
				RendererProfiles: map[string]ProfileConfig{
					"watcher-deployment-failed-detailed": {
						Source: domain.SourceWatcher,
						Key:    "watcher.deployment.failed",
						Templates: DestinationTemplates{
							Slack: &SlackTemplateConfig{
								Title: `{{ index .Payload "release" }}`,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "unknown payload path is rejected",
			cfg: Config{
				Server:   ServerConfig{Address: ":8080", MaxWebhookBodyBytes: 1},
				Database: testDatabaseConfig(),
				Workers:  testWorkersConfig(),
				Retry:    RetryConfig{MaxAttempts: 1},
				RendererProfiles: map[string]ProfileConfig{
					"watcher-deployment-failed-detailed": {
						Source: domain.SourceWatcher,
						Key:    "watcher.deployment.failed",
						Templates: DestinationTemplates{
							Slack: &SlackTemplateConfig{
								Title: "{{.Payload.release}}",
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
