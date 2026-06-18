package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/iweka-dev/webhook-hub/internal/app"
	"github.com/iweka-dev/webhook-hub/internal/clock"
	"github.com/iweka-dev/webhook-hub/internal/config"
	"github.com/iweka-dev/webhook-hub/internal/delivery"
	"github.com/iweka-dev/webhook-hub/internal/httpserver"
	"github.com/iweka-dev/webhook-hub/internal/ingress"
	ghingress "github.com/iweka-dev/webhook-hub/internal/ingress/github"
	"github.com/iweka-dev/webhook-hub/internal/ingress/watcher"
	"github.com/iweka-dev/webhook-hub/internal/observability"
	"github.com/iweka-dev/webhook-hub/internal/routing"
	"github.com/iweka-dev/webhook-hub/internal/storage/sqlite"
)

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		slog.Error("config.load_failed", "error", err)
		os.Exit(1)
	}

	logger := observability.NewLogger(cfg.Logging)
	store, err := sqlite.Open(cfg.Database, logger)
	if err != nil {
		logger.Error("database.open_failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	routeEngine := routing.New(nil)

	ingressService := ingress.NewService(
		store,
		cfg,
		ingress.NewRegistry(
			watcher.NewAdapter(),
			ghingress.NewAdapter(),
		),
		routeEngine,
		clock.Real{},
		logger,
	)
	deliveryService := delivery.NewService(store, cfg, clock.Real{}, logger)
	deliveryRunner := delivery.NewRunner(deliveryService)
	appService := app.NewService(cfg, clock.Real{}, store, ingressService, routeEngine)
	if err := appService.LoadRoutes(context.Background()); err != nil {
		logger.Error("routes.load_failed", "error", err)
		os.Exit(1)
	}

	server := httpserver.New(cfg, appService, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	deliveryRunner.Start(ctx, "worker-1")

	logger.Info("server.started", "address", cfg.Server.Address)
	if err := server.Run(); err != nil {
		logger.Error("server.stopped", "error", err)
	}
	deliveryRunner.Wait()
}
