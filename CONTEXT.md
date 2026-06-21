# Codebase Context (`CONTEXT.md`)

This document serves as the canonical orientation and traceable working context for the Webhook Notification Gateway. It details the repository structure, architecture guarantees, current state of the branch, and outstanding engineering tasks.

---

## 1. Core Purpose & Architectural Principles

The Webhook Notification Gateway is a single-node Go service that verifies source-specific webhooks, normalizes them into operational events, evaluates routing policies, persists receipts and delivery jobs in SQLite, and asynchronously delivers notifications to destination channels (Slack, Telegram, and Email).

### Core Design Constraints:
*   **Accepted Means Durable:** A webhook returns `202 Accepted` only after the receipt, normalized event, and initial delivery jobs are successfully committed to SQLite.
*   **Isolation of Network Operations:** Downstream provider network requests (HTTP posts to Slack/Telegram, SMTP calls) must never run inside SQLite transactions.
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
│   ├── routing/             # Engine matching events against additive routing selectors
│   ├── runtimeconfig/       # Streamlined in-memory hot-reload registries for configurations
│   ├── sender/              # Slack and Telegram client transports
│   └── storage/             # Persistence interfaces
│       └── sqlite/          # GORM SQLite repository, schemas, and migrations
```

---

## 3. Working Tree Status (Branch `ref`)

The working tree represents a successfully refactored codebase addressing the **Phase 7 (PRD #9)** requirements with new metadata guidelines:
*   **Compilation & Tests:** The codebase compiles and passes the entire test suite (`go test ./...` exits with code 0).
*   **Formatting:** All files are formatted according to standard Go styling guidelines.
*   **SQLite Migrations:** Fully versioned migrations (`001_initial.sql` to `008_event_routing_trace_and_payload_version.sql`) are applied on startup.
*   **Guidelines Enforcement:** Adjusted `AGENTS.md` and `CONTEXT.md` to make context tracking and ADR documentation mandatory.

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
| **Outbox Semaphore Concurrency** | `internal/delivery/runner.go` | Refactor the sequential scheduler loop to process claimed envelopes concurrently using a bounded pool based on `workers.concurrency`. |
| **Retry-After Rate Limiting** | `internal/sender/` | Extract `Retry-After` headers on Slack/Telegram 429 rate limit exceptions, scheduling `NextAttemptAt = now + Retry-After`. |
| **Sanitize Email Schema** | `internal/config/` | Explicitly label email configuration properties as incomplete or experimental until the SMTP sender is built. |
| **Operational status endpoint** | `internal/httpserver/` | Implement `/api/v1/status` to securely view configuration options and database state. |
| **Retention Worker** | `internal/delivery/` | Implement a background cleanup routine to purge historical raw payloads and delivery logs. |

---

## 6. Decision & Context Tracking Guidelines
As defined in [AGENTS.md](file:///home/fanboykun/dev/work/webhook-hub/AGENTS.md):
1. **Interactive Updates (MANDATORY):** [CONTEXT.md](file:///home/fanboykun/dev/work/webhook-hub/CONTEXT.md) (this file) MUST be updated after every interaction or set of changes to preserve a detailed, traceable state of the project.
2. **Architecture Decisions (ADRs):** Technical, design, architectural, or rule decisions must be written directly to [docs/adr/](file:///home/fanboykun/dev/work/webhook-hub/docs/adr/) as small, simple, and straightforward markdown files to keep rules and decisions simple and easy to track.
