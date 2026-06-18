# AGENTS.md

## Purpose

This repository is building the Webhook Notification Gateway: a single-node Go service that verifies source webhooks, normalizes them into operational events, persists receipts and delivery jobs durably in SQLite, and asynchronously delivers to Slack, Telegram, and email.

Future agent work should optimize for a durable vertical slice, not broad unfinished scaffolding.

## Source Of Truth

- `docs/design.md` is the canonical source of truth for architecture, runtime behavior, API semantics, persistence shape, ADRs, and version-1 scope.
- `docs/tasks.md` is the implementation tracker derived from `docs/design.md`.
- `README.md` is a compact repo/operator summary, not the canonical design reference.
- User-provided design decisions in the current repo context should be reflected back into `docs/design.md` rather than silently diverging.

If a task conflicts with these documents, preserve the architectural constraints and ask before changing them.

## Architecture Constraints

- Version 1 is one Go binary with in-process scheduler and workers.
- Accepted means receipt, events, and initial deliveries are durably committed before returning `202`.
- External network calls must never happen inside DB transactions.
- Delivery semantics are at-least-once.
- SQLite is the version-1 durable store and is owned by one active process.
- Use explicit SQL migrations; do not rely on GORM `AutoMigrate` as the production migration strategy.
- Source adapters own provider-specific parsing and verification details.
- Routing depends on normalized events, never on raw provider payload structs.
- Renderers and senders are provider-specific; routing stays provider-neutral.

## Implementation Order

Work in this order unless the user explicitly redirects:

1. Foundation: config loading/validation, logging, bootstrap, health, SQLite open/migrate, graceful shutdown.
2. Watcher to Slack vertical slice.
3. Telegram and email destinations.
4. GitHub adapter.
5. Grafana adapter.
6. Sentry adapter.
7. Operational API, retention, and hardening.

Do not start Grafana, Sentry, or GitHub work before the Watcher-to-Slack slice proves the durable ingestion and delivery boundaries.

## Repository Expectations

- Keep domain types separate from persistence models.
- Prefer small interfaces around stable boundaries: store, router, renderer registry, sender registry, clock.
- Preserve raw request bytes for signature verification where the source requires it.
- Keep source verification and normalization as separate steps.
- Use injected clocks for replay-window, retry, lease, and retention logic.
- Favor deterministic table-driven tests and `httptest` wiring over ad hoc manual verification.

## Package Direction

Target package shape:

```text
cmd/gateway
internal/app
internal/config
internal/domain
internal/httpserver
internal/ingress
internal/routing
internal/message
internal/delivery
internal/sender
internal/storage
internal/observability
internal/clock
internal/id
configs
templates
testdata
docs
```

This is a direction, not a requirement to create every package before it is needed. Prefer creating packages when a vertical slice actually uses them.

## Documentation Rules

- Keep `docs/design.md` aligned with the real intended architecture when explicit decisions change.
- Keep `README.md` high-signal and operator-facing.
- Keep `docs/tasks.md` actionable; update status, blockers, and acceptance criteria when scope changes, but do not let it drift from `docs/design.md`.
- When implementation changes an explicit decision, update `docs/design.md` first or in the same change.

## Testing Expectations

Minimum expectations for substantive implementation work:

- Unit tests for pure logic such as verification, routing, mapping, retry, and config validation.
- SQLite-backed integration tests for receipt/event/delivery persistence and lease behavior.
- HTTP tests for signature handling, duplicate handling, and admin auth where relevant.
- Worker tests using fake senders and injected clocks.

For any delivery or persistence change, report what was verified and what residual risks remain.

## Security Rules

- Never log secrets, signatures, authorization headers, cookies, Slack webhook URLs, Telegram bot tokens, or SMTP passwords.
- Never persist unauthenticated raw payloads.
- Validate destination references and secret env bindings at startup.
- Keep outbound URLs and provider endpoints configuration-owned, never payload-controlled.
- Use constant-time signature comparison and replay-window checks where defined.

## Practical Coding Guidance

- Keep transactions short and DB-only.
- Treat duplicate webhook receipts as successful `202` responses after identifying the stored duplicate condition.
- Collapse duplicate destination matches to one delivery row per `event_id` and `destination_id`.
- Preserve ignored and unrouted events for auditability instead of turning them into transport errors.
- Prefer explicit constructors returning errors; reserve `Must...` helpers for outermost startup wiring only.

## When Adding New Work

Before broadening scope, check whether the work:

- strengthens the current vertical slice,
- unlocks a blocked later phase,
- or is just premature scaffolding.

If it is premature scaffolding, do not add it unless the user asks for it directly.
