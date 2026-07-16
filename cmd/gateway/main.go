package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fanboykun/webhook-hub/internal/app"
	"github.com/fanboykun/webhook-hub/internal/clock"
	"github.com/fanboykun/webhook-hub/internal/config"
	configcrypto "github.com/fanboykun/webhook-hub/internal/config/crypto"
	"github.com/fanboykun/webhook-hub/internal/delivery"
	"github.com/fanboykun/webhook-hub/internal/httpserver"
	"github.com/fanboykun/webhook-hub/internal/ingress"
	ghingress "github.com/fanboykun/webhook-hub/internal/ingress/github"
	"github.com/fanboykun/webhook-hub/internal/ingress/watcher"
	"github.com/fanboykun/webhook-hub/internal/observability"
	"github.com/fanboykun/webhook-hub/internal/routing"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
	"github.com/fanboykun/webhook-hub/internal/storage/sqlite"
)

var Version = "dev"

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		slog.Error("config.load_failed", "error", err)
		os.Exit(1)
	}

	logger := observability.NewLogger(cfg.Logging)
	apiLogger := logger.With("component", "api")
	workerLogger := logger.With("component", "worker")
	encryptionCipher, err := configcrypto.NewFromString(cfg.Database.ResolvedEncryptionKey)
	if err != nil {
		logger.Error("config.encryption_key_invalid", "error", err)
		os.Exit(1)
	}
	store, err := sqlite.OpenWithCipher(cfg.Database, logger, encryptionCipher)
	if err != nil {
		logger.Error("database.open_failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	routeEngine := routing.New(nil)
	integrationRegistry := runtimeconfig.NewIntegrationRegistry(nil)
	destinationRegistry := runtimeconfig.NewDestinationRegistry(nil)
	rendererProfileRegistry := runtimeconfig.NewRendererProfileRegistry(nil)

	ingressService := ingress.NewService(
		store,
		cfg,
		ingress.NewRegistry(
			watcher.NewAdapter(),
			ghingress.NewAdapter(),
		),
		routeEngine,
		integrationRegistry,
		destinationRegistry,
		clock.Real{},
		apiLogger,
	)
	deliveryService := delivery.NewService(store, cfg, destinationRegistry, rendererProfileRegistry, clock.Real{}, workerLogger)
	deliveryRunner := delivery.NewRunner(deliveryService)
	appService := app.NewService(cfg, clock.Real{}, store, ingressService, routeEngine, integrationRegistry, destinationRegistry, rendererProfileRegistry)
	if err := appService.BootstrapDynamicConfig(context.Background()); err != nil {
		logger.Error("dynamic_config.bootstrap_failed", "error", err)
		os.Exit(1)
	}

	server := httpserver.New(cfg, appService, apiLogger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	deliveryRunner.Start(ctx, "worker-1")

	apiLogger.Info("server.started", "address", cfg.Server.Address, "version", Version)
	if err := server.Run(); err != nil {
		apiLogger.Error("server.stopped", "error", err)
	}
	deliveryRunner.Wait()
}
