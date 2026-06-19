# Webhook Gateway Tasks

This file turns [docs/design.md](./design.md) into trackable implementation work. If this file conflicts with the design doc, `docs/design.md` wins. Status values are:

- `todo`
- `in_progress`
- `blocked`
- `done`

## Program Rules

- Build the system as vertical slices.
- Do not broaden source support before the durable ingestion and delivery path is proven.
- Every phase must leave the repo in a runnable, testable state.
- A phase is only complete when its acceptance criteria and tests are complete.

## Phase 0: Foundation

Status: `in_progress`

Goal: bootstrap the service so code can be added against stable runtime and storage boundaries.

Tasks:

- [x] Create `cmd/gateway` startup wiring with context cancellation and graceful shutdown.
- [x] Add typed config structs, environment-secret resolution, and validation.
- [x] Initialize structured logging with `log/slog` and contextual helpers.
- [x] Bootstrap Gin with Huma/HumaGin.
- [x] Expose `/docs`, `/openapi.json`, `/health/live`, and `/health/ready`.
- [x] Open SQLite with required pragmas and safe connection settings.
- [x] Add embedded SQL migration runner and migration tracking table.
- [x] Create basic app lifecycle wiring for HTTP server startup and shutdown.

Acceptance criteria:

- [x] Valid config starts successfully.
- [ ] Invalid config fails with actionable errors.
- [x] Docs and health endpoints respond as designed.
- [x] Migrations are repeatable and idempotent.

Suggested tests:

- [ ] Config validation table tests.
- [ ] Startup integration test with temp SQLite DB.
- [x] Readiness/liveness HTTP tests.
- [x] Migration repeatability test.

## Phase 1: Durable Watcher -> Slack Slice

Status: `in_progress`

Goal: prove the end-to-end ingestion, persistence, routing, retry, and delivery model with the smallest useful source/destination pair.

Tasks:

- [x] Define the Watcher webhook contract and fixture set.
- [x] Implement Watcher HMAC verification with replay-window support.
- [x] Implement normalized event model and core domain enums.
- [x] Build routing engine with additive matching and destination deduplication.
- [x] Implement atomic receipt/event/delivery ingest flow.
- [x] Add SQLite repositories for receipts, events, deliveries, and attempts.
- [x] Implement Slack renderer.
- [x] Implement Slack sender with retry classification.
- [x] Implement scheduler, worker pool, attempt recording, and lease recovery.

Acceptance criteria:

- [x] Valid signed `deployment.failed` webhook returns `202`.
- [x] Receipt, event, and Slack delivery rows are committed atomically.
- [x] Duplicate Watcher event is treated as accepted and does not create duplicate work.
- [ ] Slack temporary failures retry with backoff.
- [ ] Slack permanent failures move to dead letter.
- [x] Pending work survives restart and is recoverable.

Suggested tests:

- [x] HMAC and replay-window unit tests.
- [ ] Ingest transaction integration test.
- [x] Duplicate receipt integration test.
- [x] Worker retry/dead-letter tests with fake Slack transport.
- [ ] End-to-end Watcher-to-Slack test with temp DB and fake HTTP receiver.

## Phase 2: Telegram And Email

Status: `todo`

Goal: extend the proven delivery path to the remaining version-1 destinations.

Tasks:

- [ ] Add Telegram renderer and sender.
- [ ] Add email renderer for text and HTML output.
- [ ] Integrate SMTP transport behind a sender interface.
- [ ] Validate email addresses and sender config at startup.
- [ ] Add destination-specific retry classification for Telegram and SMTP failures.

Acceptance criteria:

- [ ] One event can fan out independently to Slack, Telegram, and email.
- [ ] Failure in one destination does not block others.
- [ ] Secrets do not appear in logs or operational responses.

Suggested tests:

- [ ] Renderer golden tests for Telegram and email.
- [ ] Provider classification tests for Telegram and SMTP responses.
- [ ] Mixed-destination worker tests proving independent outcomes.

## Phase 3: GitHub Adapter

Status: `todo`

Goal: support high-signal GitHub event families without weakening raw-body verification or idempotency.

Tasks:

- [ ] Implement GitHub signature verification against raw body.
- [ ] Add fixtures for `pull_request`, `workflow_run`, and `release`.
- [ ] Normalize supported event families into common event types.
- [ ] Mark valid but unsupported GitHub events as ignored.
- [ ] Document route examples for GitHub workflows and releases.

Acceptance criteria:

- [ ] GitHub signature verification matches documented expectations.
- [ ] `X-GitHub-Delivery` is used as the stable receipt idempotency key.
- [ ] Unsupported but valid events return `202` and are auditable as ignored.

Suggested tests:

- [ ] Signature unit tests.
- [ ] Normalization fixture tests.
- [ ] Duplicate delivery integration test.
- [ ] HTTP tests for accepted, ignored, and rejected GitHub requests.

## Phase 4: Grafana Adapter

Status: `todo`

Goal: handle grouped alert payloads while preserving per-alert routing and atomic receipt semantics.

Tasks:

- [ ] Implement Grafana HMAC and timestamp verification.
- [ ] Parse grouped alert payloads.
- [ ] Normalize one internal event per alert instance.
- [ ] Preserve alert fingerprint and group key metadata.
- [ ] Support firing and resolved lifecycle mapping.

Acceptance criteria:

- [ ] Multi-alert receipt commits atomically.
- [ ] Replay-window violations are rejected.
- [ ] Fingerprints and group keys remain available to downstream systems.

Suggested tests:

- [ ] Timestamp and signature tests.
- [ ] Group normalization fixture tests.
- [ ] Atomic multi-event ingest integration test.

## Phase 5: Sentry Adapter

Status: `todo`

Goal: ingest high-signal issue and metric alerts without turning the service into a raw error firehose.

Tasks:

- [ ] Implement Sentry signature verification against raw body.
- [ ] Add issue-alert and metric-alert fixtures.
- [ ] Normalize triggered, regressed, and resolved event types.
- [ ] Ignore unsupported high-volume occurrence-style events by default.

Acceptance criteria:

- [ ] Raw-body signature verification is covered by tests.
- [ ] Regressed issues route distinctly from first-seen triggers.
- [ ] Unsupported but valid noisy events remain accepted as ignored.

Suggested tests:

- [ ] Signature tests.
- [ ] Fixture normalization tests.
- [ ] HTTP tests for triggered, regressed, resolved, and ignored cases.

## Phase 6: Operational API And Hardening

Status: `todo`

Goal: make the system diagnosable and operable in production-like usage.

Tasks:

- [ ] Implement event list/detail API with cursor pagination and filters.
- [ ] Implement delivery list/detail API with cursor pagination and filters.
- [x] Implement manual delivery retry endpoint.
- [ ] Add status endpoint with sanitized config/runtime visibility.
- [ ] Add retention worker for raw payloads, events, and attempts.
- [ ] Tune request, provider, and shutdown timeouts.
- [ ] Add container packaging and local deployment example.
- [ ] Document backup and restore for SQLite + WAL.

Acceptance criteria:

- [x] Operators can inspect failed deliveries and retry them.
- [ ] Retention removes expired raw payloads without breaking delivery history.
- [ ] Shutdown and restart behavior is tested.

Suggested tests:

- [ ] Admin auth tests.
- [ ] Pagination/filter tests.
- [ ] Manual retry integration test.
- [ ] Retention integration test.
- [ ] Graceful shutdown/recovery test.

## Cross-Cutting Backlog

Status: `todo`

These items should be handled within the relevant phase, not as a separate late pass.

- [ ] Keep domain structs separate from GORM models.
- [ ] Prevent secrets and sensitive headers from entering logs or persisted header snapshots.
- [ ] Use injected clocks for retry, lease, replay, and retention logic.
- [ ] Keep provider calls outside DB transactions.
- [ ] Preserve raw-body verification semantics across all webhook handlers.
- [ ] Maintain OpenAPI documentation accuracy as endpoints are added.
- [ ] Add fixture coverage for each supported source family.

## Renderer Configurability Backlog

Status: `tracked_in_github`

PRD: [Issue #1](https://github.com/fanboykun/webhook-hub/issues/1)

Goal: make render output configurable per known source and known event type without turning the renderer into an unsafe free-form template engine.

Implementation issues tracked in GitHub:
- [ ] Slice 1: Configuration Schema & Slack-only Configurable Renderer [Issue #2](https://github.com/fanboykun/webhook-hub/issues/2)
- [ ] Slice 2: Telegram Support & Variable Escaping [Issue #3](https://github.com/fanboykun/webhook-hub/issues/3)
- [ ] Slice 3: Event-Type Overrides & Golden Tests [Issue #4](https://github.com/fanboykun/webhook-hub/issues/4)

## Dynamic Config Backlog (Integrations & Destinations via API)

Status: `todo`

Goal: make webhook integrations and delivery destinations dynamically manageable via HTTP API and stored securely in SQLite.

Tasks:

- [ ] Implement AES-256-GCM secret encryption helper in `internal/config/crypto`.
- [ ] Add SQLite database migrations and GORM models for `integrations` and `destinations`.
- [ ] Implement configuration seeding from `config.yaml` to SQLite on application startup.
- [ ] Create thread-safe, hot-reloaded in-memory registries for integrations and destinations.
- [ ] Refactor Ingress service and Delivery service to resolve configurations from the dynamic registries.
- [ ] Build Huma REST CRUD API endpoints for `/api/v1/integrations` and `/api/v1/destinations` with proper validation, secret masking (redaction), and update preservation.
- [ ] Add unit and integration tests covering encryption, seeding, hot-reloading, and API endpoints.

Acceptance criteria:

- [ ] Webhook integrations and delivery destinations can be created, retrieved, updated, and deleted dynamically via the operational API.
- [ ] Sensitive fields (bot tokens, webhook URLs, SMTP passwords) are stored encrypted in SQLite.
- [ ] Sensitive fields are redacted as `"[REDACTED]"` in all retrieval API responses.
- [ ] If `GATEWAY_ENCRYPTION_KEY` is missing or has invalid length (not 32 bytes), the gateway fails to start.

## Recommended First Build Slice

Start here unless the user explicitly reprioritizes:

1. bootstrap config, logging, HTTP, health, and SQLite migration,
2. implement Watcher webhook verification,
3. normalize `deployment.failed`,
4. match one route to one Slack destination,
5. commit receipt/event/delivery atomically,
6. run one worker to send Slack and record the attempt.

That slice exercises the durable boundaries without prematurely expanding source and destination scope.
