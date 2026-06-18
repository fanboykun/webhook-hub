.DEFAULT_GOAL := help

GO ?= go
GOFMT ?= gofmt
RG ?= rg
AIR ?= air

APP_NAME ?= gateway
BIN_DIR ?= .bin
BIN_PATH ?= $(BIN_DIR)/$(APP_NAME)
CONFIG ?= config.yaml
AIR_CONFIG ?= configs/air.toml

WATCHER_WEBHOOK_SECRET ?= dev-watcher-secret
GITHUB_WEBHOOK_SECRET ?= dev-github-secret
SLACK_DEPLOYMENTS_WEBHOOK_URL ?= https://example.invalid/webhook
TELEGRAM_ONCALL_BOT_TOKEN ?= dev-telegram-token
GATEWAY_ADMIN_TOKEN ?= dev-admin-token

GO_FILES := $(shell $(RG) --files -g '*.go')

.PHONY: help fmt test build run dev check clean

help:
	@printf "Available targets:\n"
	@printf "  make fmt      Format Go source files\n"
	@printf "  make test     Run go test ./...\n"
	@printf "  make build    Build ./cmd/gateway to $(BIN_PATH)\n"
	@printf "  make run      Run the gateway with local default env\n"
	@printf "  make dev      Run Air hot reload with config $(AIR_CONFIG)\n"
	@printf "  make gen-token Generate a random admin token or shared secret\n"
	@printf "  make check    Run fmt check, tests, and build\n"
	@printf "  make clean    Remove local build artifacts\n"

fmt:
	$(GOFMT) -w $(GO_FILES)

test:
	$(GO) test ./...

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -buildvcs=false -o $(BIN_PATH) ./cmd/gateway

run:
	GATEWAY_CONFIG=$(CONFIG) \
	WATCHER_WEBHOOK_SECRET=$(WATCHER_WEBHOOK_SECRET) \
	GITHUB_WEBHOOK_SECRET=$(GITHUB_WEBHOOK_SECRET) \
	SLACK_DEPLOYMENTS_WEBHOOK_URL=$(SLACK_DEPLOYMENTS_WEBHOOK_URL) \
	TELEGRAM_ONCALL_BOT_TOKEN=$(TELEGRAM_ONCALL_BOT_TOKEN) \
	GATEWAY_ADMIN_TOKEN=$(GATEWAY_ADMIN_TOKEN) \
	$(GO) run ./cmd/gateway

dev:
	@command -v $(AIR) >/dev/null 2>&1 || { \
		printf "air is not installed. Install it with:\n"; \
		printf "  $(GO) install github.com/air-verse/air@latest\n"; \
		exit 1; \
	}
	mkdir -p tmp
	GATEWAY_CONFIG=$(CONFIG) \
	WATCHER_WEBHOOK_SECRET=$(WATCHER_WEBHOOK_SECRET) \
	GITHUB_WEBHOOK_SECRET=$(GITHUB_WEBHOOK_SECRET) \
	SLACK_DEPLOYMENTS_WEBHOOK_URL=$(SLACK_DEPLOYMENTS_WEBHOOK_URL) \
	TELEGRAM_ONCALL_BOT_TOKEN=$(TELEGRAM_ONCALL_BOT_TOKEN) \
	GATEWAY_ADMIN_TOKEN=$(GATEWAY_ADMIN_TOKEN) \
	$(AIR) -c $(AIR_CONFIG)

gen-token:
	@openssl rand -hex 32

check:
	@if [ -n "$$( $(GOFMT) -l $(GO_FILES) )" ]; then \
		printf "Unformatted Go files detected. Run 'make fmt'.\n"; \
		$(GOFMT) -l $(GO_FILES); \
		exit 1; \
	fi
	$(GO) test ./...
	$(GO) build -buildvcs=false ./cmd/gateway

clean:
	rm -rf $(BIN_DIR) tmp
