# Webhook Notification Gateway

The canonical architecture and implementation reference for this repository is [docs/design.md](./docs/design.md). This `README` is a high-signal summary for repo orientation and execution.

Webhook Notification Gateway is a single-node Go service that receives operational webhooks, verifies and normalizes them, routes them through durable policy rules, persists accepted work to SQLite, and delivers notifications to Slack, Telegram, Microsoft Teams, and email.

Version 1 is intentionally opinionated:

- One Go process.
- Gin HTTP server with Huma/HumaGin for typed operations and OpenAPI output.
- Durable SQLite inbox/outbox using explicit migrations.
- In-process scheduler and delivery workers.
- At-least-once outbound delivery.
- Configuration-defined startup defaults for integrations, destinations, renderer profiles, and routes; SQLite-backed admin changes remain authoritative after seeding.

## Problem Shape

The gateway accepts source-specific operational events from:

- Watcher
- GitHub
- Grafana Alerting
- Sentry

It then delivers routed notifications to:

- Slack
- Telegram
- Microsoft Teams
- Email

The service is not a thin proxy. A webhook is only considered accepted after the receipt, normalized events, and initial delivery jobs are durably committed.

## Core Guarantees

- Inbound webhooks are authenticated before payload processing.
- Signature verification uses the original raw request body.
- Accepted means durably persisted.
- Network calls never happen inside database transactions.
- Delivery is at-least-once, not exactly-once.
- Source parsing, routing, rendering, persistence, and transport stay independently testable.

## Version 1 Scope

Included:

- Signed webhook ingestion for Watcher, GitHub, Grafana, and Sentry.
- Normalized internal event model.
- Route matching backed by persisted live rules.
- Slack, Telegram, Microsoft Teams, and email delivery.
- Durable retries, dead-lettering, and delivery attempt history.
- Operational API for event and delivery inspection.
- Manual retry for failed deliveries.

Explicitly out of scope:

- Multi-node active/active deployment.
- Public multi-tenant SaaS behavior.
- Arbitrary user-authored transformation scripts.
- Exactly-once external delivery semantics.
- On-call schedules, acknowledgements, or bidirectional chat interactions.

## Runtime Shape

```text
Sources -> HTTP ingress -> source adapter -> normalized event batch
        -> routing engine -> SQLite receipt/event/delivery commit -> 202 Accepted

SQLite due deliveries -> scheduler -> worker pool -> renderer -> sender
                                                  -> Slack
                                                  -> Telegram
                                                  -> Microsoft Teams
                                                  -> Email
```

## Planned API Surface

Webhook ingress:

- `POST /webhooks/v1/watcher/{integration_id}`
- `POST /webhooks/v1/github/{integration_id}`
- `POST /webhooks/v1/grafana/{integration_id}`
- `POST /webhooks/v1/sentry/{integration_id}`

Operational API:

- `GET /api/v1/events`
- `GET /api/v1/events/{event_id}`
- `GET /api/v1/deliveries`
- `GET /api/v1/deliveries/{delivery_id}`
- `POST /api/v1/deliveries/{delivery_id}/retry`
- `GET /api/v1/routes`
- `GET /api/v1/routes/{route_id}`
- `POST /api/v1/routes`
- `PUT /api/v1/routes/{route_id}`
- `DELETE /api/v1/routes/{route_id}`
- `POST /api/v1/test-deliveries`
- `GET /api/v1/status`

Docs and health:

- `/docs`
- `/openapi.json`
- `/health/live`
- `/health/ready`

## Architecture Decisions

- Single binary with in-process workers keeps operations simple while preserving replaceable boundaries.
- SQLite is the durable inbox/outbox for version 1, which means one active application instance owns the database.
- Huma webhook operations are retained because raw-body verification and OpenAPI generation can coexist.
- Routing is additive; duplicate destination matches collapse into a single delivery row per event and destination.
- Built-in renderers remain the fallback baseline. Destinations can select named profiles that each target one source/event contract; templates are compiled and validated against that event's typed payload before they enter the live registry.

## Repository Direction

The detailed design, ADRs, API behavior, persistence model, and package direction live in [docs/design.md](./docs/design.md). The task breakdown is tracked in [docs/tasks.md](./docs/tasks.md), and execution guidance is in [AGENTS.md](./AGENTS.md). The first implementation target is a durable Watcher-to-Slack vertical slice that proves:

1. signed webhook verification,
2. normalization,
3. route evaluation,
4. atomic receipt/event/delivery persistence,
5. asynchronous Slack delivery,
6. retry and attempt recording.

## Configuration Principles

- Root `config.yaml` is an ignored, secret-bearing runtime file and must be protected like `.env`.
- Inline secret fields are the local default; corresponding `*_env` fields remain available for deployments and are mutually exclusive with inline values.
- Startup fails if required secrets, destinations, or integrations are invalid.
- `database.encryption_key` contains the master key used to protect dynamic integrations/destinations secrets in SQLite; `database.encryption_key_env` is the optional environment-backed form.
- SQLite runs with WAL, foreign keys enabled, a busy timeout, and short write transactions.
- A versioned migration system is required; `AutoMigrate` is not the production schema strategy.

## Secrets

- Generate `database.encryption_key` with `make gen-encryption-key`; it must decode to 32 bytes as hex or base64.
- Generate `api.admin_token` with `make gen-token`.
- Integrations use `secret` or `client_secret`; destinations use `webhook_url` or `bot_token`.
- Deployments may use the corresponding `*_env` fields instead, such as `encryption_key_env`, `admin_token_env`, `secret_env`, `webhook_url_env`, or `bot_token_env`.

## Operational Constraints

- Do not run multiple active replicas against one SQLite database file.
- Do not store or log webhook secrets, signatures, bot tokens, SMTP passwords, or provider URLs.
- Provider outages should not fail readiness while the database remains writable.
- If durable persistence is unavailable, readiness must fail and ingestion must return `503`.

## Delivery Roadmap

The implementation plan is split into trackable slices in [docs/tasks.md](./docs/tasks.md):

1. Foundation and bootstrap
2. Durable Watcher-to-Slack slice
3. Telegram and email fan-out
4. GitHub adapter
5. Grafana adapter
6. Sentry adapter
7. Operational API and hardening

## Local Commands

- `make fmt` formats all Go files.
- `make test` runs `go test ./...`.
- `make build` builds the binary to `.bin/gateway`.
- `make run` starts the service with local `config.yaml` and default dev env values.
- `make dev` starts Air hot reloading using [configs/air.toml](./configs/air.toml) and writes runtime artifacts to `tmp/air`.
- `make gen-token` prints a random 32-byte hex token for `GATEWAY_ADMIN_TOKEN` or shared webhook secrets.
- `make check` verifies formatting, tests, and buildability in one pass.

For local development, keep your untracked runtime config at `config.yaml` and use [configs/config.example.yaml](./configs/config.example.yaml) as the tracked template/reference.

## Current Status

This repository currently contains the source-of-truth design doc, repo guidance, and task breakdown. The service code should be built against the vertical-slice order above rather than attempting every integration at once.
