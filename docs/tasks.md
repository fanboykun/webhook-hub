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

## Current State

- Branch `ref` carries the latest work but the working tree is mid-refactor and **does not currently compile** (`internal/app`, `internal/httpserver`, and `cmd/gateway` fail with `undefined: runtimeconfig` / `undefined: sqlite`). Fix this before starting any new phase work. See [Phase 7](#phase-7-event-governed-redesign--prd-9) issue #12.
- The Watcher-to-Slack vertical slice is functionally complete and tested.
- Telegram destination is implemented and tested.
- GitHub adapter is implemented and tested.
- Dynamic integrations, destinations, and renderer profiles are implemented with AES-256-GCM encryption and CRUD APIs (ADR-008, superseding ADR-004).
- Email destination, Grafana adapter, Sentry adapter, retention worker, status endpoint, and container packaging remain open.
- `docs/design.md` has not yet been updated for the event-definition registry redesign tracked in [Phase 7](#phase-7-event-governed-redesign--prd-9). That doc update is part of issue #11/#12 scope.

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
- [x] Invalid config fails with actionable errors (`config.Validate` returns joined errors; `resolveSecrets` fails on missing/empty env refs).
- [x] Docs and health endpoints respond as designed.
- [x] Migrations are repeatable and idempotent.

Suggested tests:

- [ ] Config validation table tests.
- [ ] Startup integration test with temp SQLite DB.
- [x] Readiness/liveness HTTP tests.
- [x] Migration repeatability test.

Residual: config validation table tests and a startup integration test are still worth adding during [Phase 7](#phase-7-event-governed-redesign--prd-9) cleanup (issue #12).

## Phase 1: Durable Watcher -> Slack Slice

Status: `done`

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
- [x] Slack temporary failures retry with backoff.
- [x] Slack permanent failures move to dead letter.
- [x] Pending work survives restart and is recoverable.

Suggested tests:

- [x] HMAC and replay-window unit tests.
- [ ] Ingest transaction integration test.
- [x] Duplicate receipt integration test.
- [x] Worker retry/dead-letter tests with fake Slack transport.
- [ ] End-to-end Watcher-to-Slack test with temp DB and fake HTTP receiver.

Residual: an explicit ingest-transaction integration test and a full end-to-end HTTP test are still gaps. The delivery claim/lease correctness itself is being re-hardened under [Phase 7](#phase-7-event-governed-redesign--prd-9) issue #10.

## Phase 2: Telegram And Email

Status: `in_progress`

Goal: extend the proven delivery path to the remaining version-1 destinations.

Tasks:

- [x] Add Telegram renderer and sender.
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

Residual: Telegram is shipped with sender tests and retry classification. Slack `Retry-After` and Telegram `Retry-After` are not currently honored by `delivery.Service` despite design section 27 requiring it; this should be fixed alongside the email sender work or under [Phase 7](#phase-7-event-governed-redesign--prd-9). Email destination type, renderer, and sender code do not exist yet.

## Phase 3: GitHub Adapter

Status: `done`

Goal: support high-signal GitHub event families without weakening raw-body verification or idempotency.

Tasks:

- [x] Implement GitHub signature verification against raw body.
- [x] Add fixtures for `pull_request`, `workflow_run`, and `release`.
- [x] Normalize supported event families into common event types.
- [x] Mark valid but unsupported GitHub events as ignored.
- [x] Document route examples for GitHub workflows and releases.

Acceptance criteria:

- [x] GitHub signature verification matches documented expectations.
- [x] `X-GitHub-Delivery` is used as the stable receipt idempotency key.
- [x] Unsupported but valid events return `202` and are auditable as ignored.

Suggested tests:

- [x] Signature unit tests.
- [x] Normalization fixture tests.
- [ ] Duplicate delivery integration test.
- [ ] HTTP tests for accepted, ignored, and rejected GitHub requests.

Residual: duplicate-delivery and HTTP-level GitHub tests are still gaps to close during hardening.

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

Status: `in_progress`

Goal: make the system diagnosable and operable in production-like usage.

Tasks:

- [x] Implement event list/detail API with cursor pagination and filters.
- [x] Implement delivery list/detail API with cursor pagination and filters.
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

Residual: status endpoint, retention, container packaging, backup docs, Slack/Telegram `Retry-After`, `workers.concurrency`, and email completion remain open.

## Phase 7: Event-Governed Redesign — PRD #9

Status: `done`

Goal: stabilize delivery correctness, then redesign the event contract so normalized event semantics, typed payloads, and renderer profiles become centrally governed instead of scattered across adapters and generic JSON blobs.

Parent: [Issue #9 — PRD: Event-governed webhook processing and renderer contract redesign](https://github.com/fanboykun/webhook-hub/issues/9)

The PRD consolidates problems found in two review passes across five themes:

- A. Critical correctness and integrity problems (claim race, lease ownership, route JSON errors, receipt-detail truncation, weak ingest validation).
- B. Event-model and rendering contract problems (event semantics inline in adapters, `FieldsJSON` doing three jobs, renderer profiles event-aware in name only, wrong profile mental shape, persistence reinforcing weak contract).
- C. Runtime and configuration consistency problems (dynamic config write/reload split-brain, duplicated abstractions, profile validation in the wrong place, inert `workers.concurrency`).
- D. Architectural boundary and maintainability problems (`app/service.go` and `httpserver/handler.go` super files, manually repeated admin auth, HTTP layer shaping source semantics, domain file as dumping ground, duplicated mapping layers, misleading GORM tags).
- E. Scope and contract hygiene problems (email surfaced before complete, docs/tracker drift from implementation).

### Slice 7a: Delivery correctness and operational read integrity — Issue #10

Status: `done`

PRD findings addressed: A1, A2, A3, A4, A5.

Tasks:

- [x] Make `ClaimDueDeliveries` re-check eligibility in the update predicate so only rows actually won by the claimant are returned.
- [x] Make `CompleteAttempt` refuse to overwrite state when lease ownership has been lost or superseded.
- [x] Surface route JSON marshal/unmarshal failures loudly through store operations instead of degrading to zero-value routes.
- [x] Fix receipt-detail reads to return all deliveries for the receipt's events without per-event silent truncation (remove the N+1 + hardcoded cap pattern).
- [x] Strengthen ingest batch validation to reject duplicate event IDs, duplicate delivery pairs, and semantic mismatches before persistence.

Acceptance criteria:

- [x] Claiming due deliveries re-checks eligibility at update time and only returns rows actually won by the claimant.
- [x] Delivery completion refuses to overwrite state when lease ownership has already been lost or superseded.
- [x] Corrupted or incompatible persisted route JSON fails loudly through store operations instead of degrading to zero-value routes.
- [x] Receipt detail reads return all deliveries for the receipt's events without per-event silent truncation.
- [x] Store tests cover claim correctness, stale completion behavior, and route decode failures.

Blocked by: none.

### Slice 7b: Event definition registry and typed event persistence — Issue #11

Status: `done`

PRD findings addressed: B6, B7, B10, and the persistence side of the weak event contract.

Why this slice is deeper than "move code out of adapters":

- The old `domain.Event` shape tried to flatten provider-specific facts into one struct and accidentally turned source quirks into an implied platform contract.
- Fields like `service`, `environment`, `release`, `commit_sha`, `actor`, and `url` look universal, but they are not guaranteed by every source, and they mean different things across sources.
- That creates a false promise to the rest of the system: renderers, routes, APIs, and persistence start behaving as if every source can always supply the same dimensions.
- Once those fields are stored directly on the event row, the database reinforces the mistake and every new source gets pressured to "fit" the existing shape instead of declaring its own event contract honestly.
- The practical failure mode is semantic erosion: adapters invent best-effort mappings, payload-specific detail leaks into generic fields, and missing concepts get hidden instead of modeled.
- The fix is not merely a registry. The real target is an event envelope that contains only stable cross-source concepts, plus a versioned typed payload owned by the event definition for everything source-specific.

Tasks:

- [x] Introduce a central event-definition registry owning event keys, source binding, payload version, payload schema/view contract, and normalized envelope projection rules.
- [x] Refactor Watcher and GitHub adapters to emit registry-known event candidates rather than fully owning normalized semantics inline.
- [x] Persist a normalized event envelope separately from a versioned typed payload (keep the normalized event key in `type`, treat legacy `fields_json` as metadata, and add `payload_version` plus `payload_json`).
- [x] Stop writing routing outcome metadata back into the same blob as source event payload data.
- [x] Update `docs/design.md` section 17 (Normalized Event Model) and section 23 (Persistence Model) to reflect the envelope + typed payload split.
- [x] Recompose `domain.Event` so it stops advertising source-shaped fields as a universal contract.
- [x] Define the minimal stable normalized envelope explicitly and move source-owned dimensions behind typed payload and metadata accessors.
- [x] Remove direct renderer, routing, and HTTP dependence on provider-ish event fields that are not guaranteed cross-source.
- [x] Rename or wrap ambiguous envelope fields where necessary so the contract reads like event-platform language rather than GitHub/Watcher carry-over language.
- [x] Add round-trip tests that prove a source can omit provider-specific concepts without inventing fake values just to satisfy `domain.Event`.

Acceptance criteria:

- [x] A central registry defines supported normalized event keys, source binding, payload version, and normalized projection rules.
- [x] Watcher and GitHub adapters emit registry-known event candidates rather than fully owning normalized semantics inline.
- [x] Event persistence stores normalized envelope data separately from typed payload data.
- [x] Routing outcome metadata is no longer written back into the same blob as source event payload data.
- [x] Registry and persistence changes are covered by tests for key validation, projection, and round-trip storage.
- [x] `domain.Event` no longer requires adapters to squeeze source-specific concepts into a flat pseudo-universal shape.
- [x] The event envelope contains only cross-source semantics that routing, operations, and fallback rendering can actually rely on.
- [x] Source-specific render data is obtained through typed payload handling rather than through flat convenience fields on the event row.
- [x] Adding a new source no longer requires extending the generic event envelope just to preserve honest provider meaning.

Blocked by: [Issue #10](https://github.com/fanboykun/webhook-hub/issues/10).

### Slice 7c: App/HTTP boundary cleanup and runtime config reliability — Issue #12

Status: `done`

PRD findings addressed: C11, C13, D15, D16, D17, D19, E23. Also unblocks the current build break on branch `ref`.

Tasks:

- [x] Fix the build break on branch `ref` (`internal/app/service.go` references `runtimeconfig` and `sqlite` without compiling; restore the working tree to a green `go test ./...`).
- [x] Split `internal/app/service.go` by responsibility (bootstrap/reload, auth, CRUD per resource, validation, operational reads) without changing public behavior.
- [x] Split `internal/httpserver/handler.go` by endpoint family (webhooks, health, receipts, deliveries, integrations, destinations, renderer profiles, routes) without changing existing route behavior.
- [x] Make admin authorization structural for `/api/v1/` endpoints instead of manually repeated in each handler (wire the existing `adminAuthMiddleware` or replace the per-handler `Authorize` calls).
- [x] Make runtime config reload failures distinguishable from validation/persistence failures so operators can tell whether data persisted and what needs reconciliation.
- [x] Move renderer-profile validation out of startup config code into the profile model/service boundary.
- [x] Reconcile this file and `docs/design.md` with real implemented behavior and remaining scope.

Acceptance criteria:

- [x] App orchestration is split by responsibility without changing public behavior.
- [x] HTTP handlers are split by endpoint family without changing existing route behavior.
- [x] Admin authorization is enforced structurally for admin endpoints.
- [x] Runtime config write-success and reload-failure is distinguishable from validation or persistence failure.
- [x] Design and task docs are updated to match real implemented behavior and remaining scope.

Blocked by: [Issue #10](https://github.com/fanboykun/webhook-hub/issues/10).

### Slice 7d: Event-aware renderer profiles — Issue #13

Status: `done`

PRD findings addressed: B8, B9, and the rendering side of the weak event contract.

Tasks:

- [x] Redesign renderer profiles around event definitions so template authoring matches the actual event contract.
- [x] Make templates receive event-aware payload data (from the typed payload introduced in issue #11) in addition to the normalized envelope fields.
- [x] Tie profile validation to the event-definition registry rather than a flat list of event type strings.
- [x] Keep existing fallback rendering behavior available when no event-specific template is configured.
- [x] Update the renderer-profile API and OpenAPI docs to reflect the new event-aware authoring model.

Acceptance criteria:

- [x] Renderer profiles are modeled and validated against known event definitions.
- [x] Templates can access event-specific payload fields in addition to normalized envelope fields.
- [x] Existing fallback rendering behavior remains available when no event-specific template is configured.
- [x] The renderer-profile API and docs reflect the new event-aware authoring model.
- [x] Renderer tests cover both event-specific payload access and fallback behavior.

Blocked by: [Issue #11](https://github.com/fanboykun/webhook-hub/issues/11).

## Cross-Cutting Backlog

Status: `in_progress`

These items should be handled within the relevant phase, not as a separate late pass.

- [x] Keep domain structs separate from GORM models.
- [x] Prevent secrets and sensitive headers from entering logs or persisted header snapshots.
- [x] Use injected clocks for retry, lease, replay, and retention logic.
- [x] Keep provider calls outside DB transactions.
- [x] Preserve raw-body verification semantics across all webhook handlers.
- [x] Maintain OpenAPI documentation accuracy as endpoints are added (admin auth is enforced structurally through middleware and documented through the bearer security scheme).
- [x] Add fixture coverage for each supported source family.
- [ ] Honor provider `Retry-After` for Slack 429 and Telegram 429 (design section 27; currently ignored by `delivery.Service`).
- [ ] Resolve the inert `workers.concurrency` config (implement a real bounded worker pool or remove the field until needed).
- [ ] Remove or explicitly mark email as incomplete across domain/config/renderer schema surfaces until the email sender ships (PRD finding E22).

## Renderer Configurability Backlog

Status: `done`

PRD: [Issue #1](https://github.com/fanboykun/webhook-hub/issues/1)

Goal: make render output configurable per known source and known event type without turning the renderer into an unsafe free-form template engine.

Implementation issues (all closed):
- [x] Slice 1: Configuration Schema & Slack-only Configurable Renderer [Issue #2](https://github.com/fanboykun/webhook-hub/issues/2)
- [x] Slice 2: Telegram Support & Variable Escaping [Issue #3](https://github.com/fanboykun/webhook-hub/issues/3)
- [x] Slice 3: Event-Type Overrides & Golden Tests [Issue #4](https://github.com/fanboykun/webhook-hub/issues/4)

Note: the current source-first profile shape is functional but will be redesigned to event-first under [Phase 7](#phase-7-event-governed-redesign--prd-9) issue #13.

## Dynamic Config Backlog (Integrations & Destinations via API)

Status: `done`

PRD: [Issue #5](https://github.com/fanboykun/webhook-hub/issues/5)

Goal: make webhook integrations and delivery destinations dynamically manageable via HTTP API and stored securely in SQLite.

Implementation issues (all closed):
- [x] [Issue #6](https://github.com/fanboykun/webhook-hub/issues/6) — Dynamic Webhook Ingress for Integrations (End-to-End)
- [x] [Issue #7](https://github.com/fanboykun/webhook-hub/issues/7) — Dynamic Slack Delivery Destination (End-to-End)
- [x] [Issue #8](https://github.com/fanboykun/webhook-hub/issues/8) — Refactor Route Validation for Dynamic Destinations

Shipped:
- [x] AES-256-GCM secret encryption helper in `internal/config/crypto`.
- [x] SQLite migrations and GORM models for `integrations`, `destinations`, and `renderer_profiles`.
- [x] Configuration seeding from `config.yaml` to SQLite on application startup.
- [x] Thread-safe, hot-reloaded in-memory registries for integrations, destinations, and renderer profiles.
- [x] Ingress service and delivery service resolve configurations from the dynamic registries.
- [x] Huma REST CRUD API for `/api/v1/integrations`, `/api/v1/destinations`, and `/api/v1/renderer-profiles` with validation, secret masking, and update preservation.
- [x] Huma REST CRUD API for `/api/v1/renderer-profiles`.

Acceptance criteria (met):
- [x] Webhook integrations and delivery destinations can be created, retrieved, updated, and deleted dynamically via the operational API.
- [x] Sensitive fields are stored encrypted in SQLite using AES-256-GCM.
- [x] Sensitive fields are redacted as `"[REDACTED]"` in all retrieval API responses.
- [x] `database.encryption_key_env` points to a present and valid master key (32 decoded bytes as hex or base64).

Residual: the write-then-reload split-brain window (PRD finding C11) is addressed under [Phase 7](#phase-7-event-governed-redesign--prd-9) issue #12.

## Recommended First Build Slice

Status: `done`

Start here unless the user explicitly reprioritizes:

1. bootstrap config, logging, HTTP, health, and SQLite migration,
2. implement Watcher webhook verification,
3. normalize `deployment.failed`,
4. match one route to one Slack destination,
5. commit receipt/event/delivery atomically,
6. run one worker to send Slack and record the attempt.

That slice exercises the durable boundaries without prematurely expanding source and destination scope. It is complete; subsequent work follows [Phase 7](#phase-7-event-governed-redesign--prd-9) for stabilization and the event-contract redesign, then resumes [Phase 2](#phase-2-telegram-and-email) (email), [Phase 4](#phase-4-grafana-adapter), and [Phase 5](#phase-5-sentry-adapter).
