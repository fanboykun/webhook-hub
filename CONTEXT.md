# Codebase Context (`CONTEXT.md`)

This document serves as the canonical orientation and traceable working context for the Webhook Notification Gateway. It details the repository structure, architecture guarantees, current state of the branch, and outstanding engineering tasks.

---

## 1. Core Purpose & Architectural Principles

The Webhook Notification Gateway is a single-node Go service that verifies source-specific webhooks, normalizes them into operational events, evaluates routing policies, persists receipts and delivery jobs in SQLite, and asynchronously delivers notifications to destination channels (Slack, Telegram, Microsoft Teams, and Email).

### Core Design Constraints:
*   **Accepted Means Durable:** A webhook returns `202 Accepted` only after the receipt, normalized event, and initial delivery jobs are successfully committed to SQLite.
*   **Isolation of Network Operations:** Downstream provider network requests (HTTP posts to Slack/Telegram/Teams, SMTP calls) must never run inside SQLite transactions.
*   **At-Least-Once Delivery:** Jobs are claimed via short-duration leases. Temporary failures undergo backoff-retries, while leases are recovered if a worker crashes.

---

## 2. Package Map & Responsibilities

The package architecture follows a strict layout designed to isolate ingestion, storage, routing, and delivery interfaces:

```text
├── cmd/
│   └── gateway/             # Application entry point (main.go), bootstraps server and workers
├── internal/
│   ├── app/                 # Service orchestration layer (split by responsibility)
│   │   ├── auth.go          # Bearer auth validation logic
│   │   ├── destinations.go  # CRUD for delivery destinations
│   │   ├── dynamic_config.go# Dynamic seeding and reloading from YAML config to SQLite
│   │   ├── integrations.go  # CRUD for webhook integrations
│   │   ├── receipt_delivery.go # Receipt retrieval, page filters, and retry triggers
│   │   ├── renderer_profiles.go # CRUD for template-based profiles
│   │   ├── routes.go        # CRUD for routing engine policies
│   │   └── service.go       # Core gateway coordinator definition
│   ├── clock/               # Mockable clock package for time injection
│   ├── config/              # Configuration schemas, YAML parser, and secrets resolution
│   │   └── crypto/          # AES-256-GCM cipher helper for protecting secrets at rest
│   ├── delivery/            # Outbox delivery runner, polling scheduler, and retry math
│   ├── domain/              # Clean core business types (receipts, events, deliveries, attempts)
│   ├── eventcatalog/        # Central event definition catalog and projection engine
│   │   ├── watcher/         # Watcher source events projection rules
│   │   └── github/          # GitHub source events projection rules
│   ├── httpserver/          # REST operational API and webhook handlers (Huma + Gin)
│   ├── id/                  # Lexicographically sortable ULID generator
│   ├── ingress/             # Webhook validation, signature checks, and adapter registry
│   ├── message/             # Custom template-driven and fallback message renderers
│   ├── observability/       # Structured slog JSON/Text initialization
│   ├── renderprofile/       # Strict event-contract template compiler
│   ├── routing/             # Engine matching events against additive routing selectors
│   ├── runtimeconfig/       # Streamlined in-memory hot-reload registries for configurations
│   ├── sender/              # Slack, Telegram, and Teams client transports
│   └── storage/             # Persistence interfaces
│       └── sqlite/          # GORM SQLite repository, schemas, and migrations
```

---

## 3. Working Tree Status (Branch `main`)

The working tree contains the production-hardening correction for the **Phase 7 (PRD #9)** renderer and event contracts:
*   **Compilation & Tests:** The codebase compiles and the complete race suite passes (`go test -race ./...`).
*   **Formatting:** All files are formatted according to standard Go styling guidelines.
*   **SQLite Migrations:** Fully versioned migrations (`001_initial.sql` to `009_reset_unshipped_renderer_contract.sql`) are applied on startup. Migration 009 intentionally clears unshipped notification, route, destination, and renderer-profile data so the incompatible old profile shape cannot survive an upgrade; configured defaults repopulate the empty dynamic tables during the same startup.
*   **Migration 009 Review:** Migration 009 contains no schema change; it only purges development data written under the discarded renderer contract. Because the service is unshipped and clean database recreation is acceptable, it is a removal candidate rather than a required production migration. Removal is pending confirmation and must include the ADR/design references; any developer database that already recorded 009 should also be recreated before its version number is reused.
*   **Guidelines Enforcement:** Adjusted `AGENTS.md` and `CONTEXT.md` to make context tracking and ADR documentation mandatory.
*   **Release Workflow:** Issue #15 is committed in `5418fdb`; it produces a Windows amd64 archive and Watcher `version.json` manifest. ADR-0009 records the release contract.
*   **Event Contracts:** Watcher definitions are event-specific. Ingress enforces payload version, required paths, JSON types, unknown-field rejection, and absolute HTTP(S) source links before persistence; optional fields are materialized deterministically for rendering.
*   **Renderer Profiles:** Issues #13 and #16 remain open as requested, but the local implementation now uses destination profile lists with one uniquely named profile per source/event. Profiles compile once through `internal/renderprofile`, reject dynamic template bypasses, escape provider-controlled values, and enforce provider output limits. ADR-0011 and ADR-0012 record the model.
*   **Dynamic Safety:** Typed integration, destination, renderer-profile, and route defaults seed their corresponding empty SQLite tables in one startup transaction. Non-empty tables remain operator-owned. Bootstrap validates and publishes all persisted registries plus routes before HTTP or workers start; invalid persisted routes leave the previous routing snapshot untouched. CRUD writes are serialized; referenced profiles and destinations return `409`; active deliveries prevent destination deletion; historical deliveries cannot be retried after destination removal. ADR-0013 records this contract.
*   **Worker Concurrency:** `workers.concurrency` starts the configured number of independently leased schedulers, with one separate lease-recovery loop.
*   **Current Verification:** On 2026-07-16, `go test -race ./...`, `go vet ./...`, `git diff --check`, and the Windows amd64 version-stamped build passed. The Windows build emitted only the known read-only Go module stat-cache warning.

---

## 4. Recomposed Design Elements (Reconciling PRD #9)

### Theme A: Correctness & Lock Safety (Issue #10)
*   `ClaimDueDeliveries` implements a single-row CAS (Compare-And-Swap) predicate check using `RowsAffected` verification to prevent claim races between workers.
*   `CompleteAttempt` verifies lease ownership matching the `locked_by` token and status before updating attempts, preventing stale lease overwrites.
*   `GetReceipt` performs batch queries of deliveries using event IDs, eliminating N+1 querying and truncation.
*   `validateIngestBatch` provides strict constraints checking for duplicate event IDs or target deliveries prior to write transactions.

### Theme B: Event-Model & Persistence Contract (Issue #11)
*   The ad hoc `fields_json` column is decomposed. The `event` schema now persists:
    *   `payload_json` (raw typed JSON representing the specific source event payload).
    *   `payload_version` (integer identifier of the event payload schema version).
    *   `route_trace_json` (routing selectors metadata containing matching rules and route IDs, decoupled from payload).

### Theme C: Boundary Separation & HTTP Sanitization (Issue #12 & #13)
*   Ingress adapters delegate normalization logic to the `internal/eventcatalog` projectors, making verification and normalization separate.
*   Admin authentication is centralized in the Gin pipeline (`adminAuthMiddleware`).
*   Config reload failures are caught at the service level, joining with `ErrRuntimeReloadRequired` to inform HTTP clients that writes succeeded but live registry updates failed.

---

## 5. Traceable Remaining Backlog

These items represent the outstanding tasks required to complete Version 1 development, mapped against the backlog trackers:

| Task | Target Package | Goal / Requirement |
|---|---|---|
| **Moving Profile Validation** | `internal/app/validation.go` | Shift profile check out of `internal/config/config.go` to eliminate coupling between parsing and the event registry catalog. |
| **Retry-After Rate Limiting** | `internal/sender/` | Extract `Retry-After` headers on Slack/Telegram/Teams 429 rate limit exceptions, scheduling `NextAttemptAt = now + Retry-After`. |
| **Sanitize Email Schema** | `internal/config/` | Explicitly label email configuration properties as incomplete or experimental until the SMTP sender is built. |
| **Operational status endpoint** | `internal/httpserver/` | Implement `/api/v1/status` to securely view configuration options and database state. |
| **Retention Worker** | `internal/delivery/` | Implement a background cleanup routine to purge historical raw payloads and delivery logs. |

## 6. GitHub Issue Sweep On 2026-07-16

Open issues inspected:

*   **#9 PRD: Event-governed webhook processing and renderer contract redesign** — local docs and code show the PRD slices completed.
*   **#10 Delivery correctness and operational read integrity** — local implementation has claim CAS, lease-owned completion, loud route JSON decode, receipt fan-out reads, and store tests.
*   **#11 Event definition registry and typed event persistence** — local implementation has `internal/eventcatalog`, adapter projection, payload versioning, `payload_json`, and `route_trace_json`.
*   **#12 App/HTTP boundary cleanup and runtime config reliability** — local implementation splits app and HTTP responsibilities, centralizes admin auth middleware, and distinguishes reload-required errors.
*   **#13 Event-aware renderer profiles** — remains open for user review. Local corrective work now models destination profile lists with one named profile per source/event, enforces event payload contracts, and precompiles restricted templates.
*   **#15 Create Release Workflow** — implemented with Watcher-compatible release packaging and manifest generation.
*   **#16 add microsoft teams webhook destinations** — remains open for user review. Teams templates now use the same corrected event-specific profile compiler, recursive escaping, output limits, and destination/profile validation as Slack and Telegram.

Branch audit after the issue sweep:

*   Current checkout is `main` at `5418fdb`, matching `origin/main`.
*   No local branches have commits missing from `main` (`git branch --no-merged main` is empty).
*   `codex-dynamic-integrations-destinations`, `feat/configurable-renderer`, `feat/configurable-renderer-apis`, and `subagent-Configurable-Renderer-Developer-self-eec407a4` are all merged ancestors of `main`.
*   The `feat/configurable-renderer-apis` auxiliary worktree has one uncommitted edit in `internal/app/service.go` adding old renderer profile list/preview helpers. That edit is based on the pre-redesign service shape and does not directly apply to current `main` without redesign.
*   Issue #15 release workflow changes are already committed in `5418fdb` and should remain closed. Corrective renderer-profile work is currently uncommitted local working-tree state on top of `main`.

---

## 7. Decision & Context Tracking Guidelines
As defined in [AGENTS.md](file:///home/fanboykun/dev/work/webhook-hub/AGENTS.md):
1. **Interactive Updates (MANDATORY):** [CONTEXT.md](file:///home/fanboykun/dev/work/webhook-hub/CONTEXT.md) (this file) MUST be updated after every interaction or set of changes to preserve a detailed, traceable state of the project.
2. **Architecture Decisions (ADRs):** Technical, design, architectural, or rule decisions must be written directly to [docs/adr/](file:///home/fanboykun/dev/work/webhook-hub/docs/adr/) as small, simple, and straightforward markdown files to keep rules and decisions simple and easy to track.
