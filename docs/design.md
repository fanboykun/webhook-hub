# Webhook Notification Gateway

**Technical Design and Implementation Plan**  
**Status:** Draft for implementation  
**Runtime shape:** Single-node Go service with durable in-process workers  
**Primary sources:** Watcher, GitHub, Grafana Alerting, Sentry  
**Primary destinations:** Slack, Telegram, Email

---

## 1. Executive Summary

The Webhook Notification Gateway receives operational events from several systems, verifies and normalizes each source-specific payload, evaluates routing policies, and delivers destination-specific messages to Slack, Telegram, and email.

The gateway is intentionally more than a thin HTTP proxy. It persists accepted webhook receipts and delivery jobs before returning success, allowing retries, delivery history, deduplication, and recovery after process restarts.

The first implementation is designed as one Go process containing:

- A Gin HTTP server.
- Huma with the HumaGin adapter for typed operations and generated OpenAPI documentation.
- Source-specific webhook adapters.
- A normalized operational event model.
- Configuration-defined routing policies.
- A durable SQLite inbox/outbox implemented through GORM repositories.
- Background delivery workers for Slack, Telegram, and email.
- Structured contextual logging using the standard `log/slog` package.

The service provides **at-least-once delivery**, not exactly-once delivery. A provider may accept a message immediately before the gateway crashes, resulting in a duplicate when the delivery lease expires and is retried. The design minimizes duplicates through source idempotency keys and durable state, but it does not claim an impossible cross-provider exactly-once guarantee.

---

## 2. Problem Context

The gateway receives webhooks from:

| Source | Responsibility |
|---|---|
| Watcher | Internal deployment lifecycle and deployment results |
| GitHub | Repository, pull request, workflow, release, and selected security activity |
| Grafana Alerting | Backend, infrastructure, and service-health alerts |
| Sentry | Frontend and application error, regression, and performance alerts |

It sends notifications to:

| Destination | Primary use |
|---|---|
| Telegram | Urgent and compact on-call notifications |
| Slack | Rich operational messages, engineering history, and team-specific channels |
| Email | Durable notifications, critical escalations, and later digest delivery |

These systems do not share the same authentication method, payload shape, event identifiers, lifecycle, or message capabilities. The gateway must preserve source-specific meaning without leaking source-specific payloads into destination implementations.

---

## 3. Goals

1. Authenticate every inbound webhook before processing its payload.
2. Preserve raw request bytes for signature verification.
3. Normalize source-specific payloads into one internal operational event model.
4. Route events using deterministic configuration-defined policies.
5. Persist accepted receipts, normalized events, and delivery jobs atomically.
6. Return a successful response only after the work is durably recorded.
7. Deliver independently to Slack, Telegram, and email.
8. Retry temporary failures with exponential backoff and jitter.
9. Prevent duplicate processing of retried source deliveries when a stable source delivery ID is available.
10. Provide delivery history and manual retry through a documented operational API.
11. Remain easy to run as a single binary with a local SQLite database.
12. Keep source, routing, rendering, persistence, and transport concerns independently testable.

---

## 4. Non-Goals for Version 1

- A multi-tenant public SaaS product.
- Horizontal scaling across multiple active gateway replicas.
- A graphical configuration or template editor.
- Arbitrary user-authored scripts for payload transformation.
- Exactly-once delivery across external providers.
- Bidirectional Slack or Telegram interactions.
- Alert acknowledgement and escalation workflows.
- On-call schedule management.
- Full replacement for Grafana, Sentry, or GitHub notification rules.
- A general-purpose event-streaming platform.
- Dynamic destination URLs supplied by incoming payloads.
- Email bounce and complaint processing.
- Email digests in the first vertical slice.

---

## 5. Architectural Principles

### 5.1 Source adapters own source knowledge

Watcher, GitHub, Grafana, and Sentry payloads are parsed only inside their corresponding source adapters. Routing and destination code must never depend directly on the original provider structs.

### 5.2 Normalized events carry operational meaning

The normalized event model preserves source identifiers and raw metadata while exposing common concepts such as severity, lifecycle, service, environment, release, and fingerprint.

### 5.3 Routing is separate from rendering

Routing decides **where** an event goes. Rendering decides **how** the event appears on a destination.

### 5.4 Accepted means durable

The gateway returns `202 Accepted` only after the receipt, normalized events, and initial delivery rows are committed to SQLite.

### 5.5 Network calls never occur inside database transactions

Database transactions must remain short. Slack, Telegram, SMTP, DNS, and any other network operation happen only after a transaction is committed.

### 5.6 Configuration references secrets

Configuration contains environment-variable references, not plaintext tokens, webhook URLs, SMTP passwords, or signing secrets.

### 5.7 One process, replaceable boundaries

Version 1 runs HTTP ingestion and delivery workers in one process. Interfaces must allow the worker, database, or transport adapters to be split later without changing the domain model.

---

## 6. High-Level Architecture

```text
Watcher ─────┐
GitHub ──────┤
Grafana ─────┼──► HTTP Ingress ─► Source Adapter ─► Normalized Event Batch
Sentry ──────┘           │                │                    │
                         │                │                    ▼
                         │                │              Routing Engine
                         │                │                    │
                         │                └─────────────┐      ▼
                         │                              │  SQLite Transaction
                         │                              │  - webhook receipt
                         │                              │  - normalized events
                         │                              │  - delivery jobs
                         │                              │
                         ◄────────── 202 Accepted ──────┘

Delivery Poller ─► Claim Due Jobs ─► Worker Pool ─► Renderer ─► Sender
                                                        ├──► Slack
                                                        ├──► Telegram
                                                        └──► Email
```

### Runtime components

1. **HTTP server** — Gin router, middleware, Huma API, webhook operations, health endpoints.
2. **Integration registry** — validated in-memory configuration for ingress integrations.
3. **Source adapter registry** — maps source names to adapter implementations.
4. **Ingestion service** — verification, normalization, idempotency, persistence, and routing orchestration.
5. **Routing engine** — evaluates normalized events against ordered route policies.
6. **SQLite store** — durable inbox, event journal, delivery outbox, and attempt history.
7. **Delivery scheduler** — claims due delivery jobs using short leases.
8. **Worker pool** — renders and sends messages outside database transactions.
9. **Destination registry** — resolves named Slack, Telegram, and email destinations.
10. **Operational API** — event history, delivery history, delivery details, manual retry, health.
11. **Housekeeping worker** — recovers expired leases and applies retention policies.

---

## 7. Selected Technology Stack

### 7.1 Go

Use a currently supported Go release and pin it in `go.mod`. The design requires a Go version containing `log/slog`.

### 7.2 Gin

Gin is the underlying HTTP router and middleware host. Use `gin.New()` instead of `gin.Default()` so logging and recovery middleware are controlled explicitly.

### 7.3 Huma and HumaGin

Huma supplies typed input/output models, validation, OpenAPI 3.1 generation, and interactive API documentation. HumaGin connects Huma to the Gin router.

Huma supports a `RawBody []byte` input field, so webhook operations can remain documented Huma operations while still verifying signatures against the exact request bytes.

Recommended paths:

```text
/docs             interactive API documentation
/openapi.json     generated OpenAPI document
/api/v1/...       operational API
/webhooks/v1/...  provider webhook ingress
/health/live      liveness
/health/ready     readiness
```

### 7.4 Viper

Viper loads YAML configuration and environment overrides. Configuration is loaded and validated once during startup. Integration credentials, transport settings, and other process-owned configuration remain startup-loaded in version 1. Live route changes are supported through the operational API and persisted in SQLite.

### 7.5 `log/slog` with contextual helpers

Use the standard structured logger. A small internal `logctx` package stores and derives loggers from `context.Context`.

Expected correlation attributes include:

```text
request_id
source
integration_id
receipt_id
event_id
source_event_id
delivery_id
destination_id
attempt
worker_id
```

### 7.6 GORM and SQLite

GORM implements repository persistence. Domain structs remain separate from GORM models.

SQLite provides the durable inbox/outbox for the single-node deployment. Use:

- WAL journal mode.
- Foreign keys enabled.
- A busy timeout.
- Short write transactions.
- A local persistent filesystem, never a shared network filesystem.
- One application instance owning the database.

A safe version-1 default is one open database connection. This serializes database access and avoids many lock-contention surprises. It is configurable for later testing, but increasing it does not allow multiple concurrent SQLite writers.

Use explicit embedded SQL migrations rather than relying on `AutoMigrate` as the production schema-management strategy.

### 7.7 HTTP clients and email

- Slack: standard `net/http` client posting to configured app-based incoming webhook URLs.
- Telegram: standard `net/http` client calling the Telegram Bot API.
- Email: an SMTP transport behind an interface. Prefer a maintained SMTP/MIME library rather than building directly on the frozen standard-library `net/smtp` package.

All outbound HTTP clients must define connection, request, and idle timeouts.

---

## 8. HTTP API Design

## 8.1 Webhook endpoints

```text
POST /webhooks/v1/watcher/{integration_id}
POST /webhooks/v1/github/{integration_id}
POST /webhooks/v1/grafana/{integration_id}
POST /webhooks/v1/sentry/{integration_id}
```

An integration ID selects a configured source instance and its secret. It is an identifier, not a credential.

### Successful response

```json
{
  "receipt_id": "01J...",
  "status": "accepted",
  "duplicate": false,
  "event_count": 2,
  "delivery_count": 4
}
```

Use `202 Accepted` for both a newly accepted receipt and a recognized duplicate. Returning success for duplicates prevents a source from repeatedly retrying a delivery already recorded by the gateway.

### Response behavior

| Condition | Response |
|---|---:|
| Accepted and durably committed | `202` |
| Duplicate source delivery | `202` |
| Recognized but intentionally ignored event | `202` |
| Missing or invalid signature | `401` |
| Unknown integration | `404` |
| Unsupported media type | `415` |
| Body exceeds configured limit | `413` |
| Malformed source payload | `400` |
| Persistence unavailable | `503` |
| Internal processing error | `500` |

Do not return a non-2xx response merely because an event is not routed. The receipt can be persisted as `unrouted` and inspected later.

## 8.2 Operational API

```text
GET  /api/v1/events
GET  /api/v1/events/{event_id}
GET  /api/v1/deliveries
GET  /api/v1/deliveries/{delivery_id}
POST /api/v1/deliveries/{delivery_id}/retry
GET  /api/v1/routes
GET  /api/v1/routes/{route_id}
POST /api/v1/routes
PUT  /api/v1/routes/{route_id}
DELETE /api/v1/routes/{route_id}
POST /api/v1/test-deliveries
GET  /api/v1/status
```

The operational API must be protected independently from source webhook authentication. Version 1 may use a static bearer token referenced through an environment variable and constant-time comparison. It should be placed behind a private network or authenticated reverse proxy when possible.

### Suggested filters

`GET /api/v1/events`:

```text
source
type
severity
lifecycle
service
environment
from
to
limit
cursor
```

`GET /api/v1/deliveries`:

```text
status
destination_id
event_id
from
to
limit
cursor
```

Use cursor pagination rather than offset pagination.

## 8.3 Health endpoints

### `/health/live`

Returns success while the process and HTTP server are alive. It does not query the database.

### `/health/ready`

Returns success only when:

- Configuration loaded and validated.
- SQLite is reachable and migrated.
- The delivery scheduler has started.
- Required destination configuration is present.

Provider reachability should not normally make readiness fail because a temporary Slack, Telegram, or SMTP outage should not remove the gateway from service; durable retries are designed for that condition.

---

## 9. Huma Webhook Input Pattern

Use a separate Huma operation per source so the generated documentation shows the correct source headers.

Example GitHub input:

```go
type GitHubWebhookInput struct {
    IntegrationID string `path:"integration_id"`
    Event         string `header:"X-GitHub-Event"`
    DeliveryID    string `header:"X-GitHub-Delivery"`
    Signature     string `header:"X-Hub-Signature-256"`
    RawBody       []byte `contentType:"application/json"`
}
```

Watcher, Grafana, and Sentry receive their own input types.

For the internally controlled Watcher payload, Huma may use both a typed `Body` and `RawBody`:

```go
type WatcherWebhookInput struct {
    IntegrationID string `path:"integration_id"`
    WebhookID     string `header:"webhook-id"`
    Timestamp     string `header:"webhook-timestamp"`
    Signature     string `header:"webhook-signature"`
    Event         string `header:"X-Watcher-Event"`
    DeliveryID    string `header:"X-Watcher-Delivery-ID"`
    Body          WatcherPayload
    RawBody       []byte
}
```

For externally controlled payloads, prefer `RawBody` plus tolerant adapter-owned decoding. Strict boundary schemas for GitHub, Grafana, and Sentry can break when providers add fields or vary payloads by event type.

---

## 10. Inbound Security Model

### 10.1 General controls

Every webhook endpoint must apply:

- HTTPS at the service or reverse proxy.
- Per-route maximum body size; default recommendation: 1 MiB.
- Strict `Content-Type` handling.
- Signature verification before JSON decoding where the source supports signatures.
- Constant-time signature comparison.
- Optional source-specific timestamp replay checks.
- Request timeout.
- Trusted-proxy configuration; never trust arbitrary forwarded headers.
- Minimal logging before authentication succeeds.
- No persistence of unauthenticated payload bodies.

### 10.2 Watcher

Recommended headers:

```text
webhook-id
webhook-timestamp
webhook-signature
X-Watcher-Event
X-Watcher-Delivery-ID
```

Signature input:

```text
HMAC-SHA256(secret, webhook-id + "." + webhook-timestamp + "." + raw_body)
```

Reject timestamps outside the configured replay window, recommended default: five minutes.

### 10.3 GitHub

Use:

```text
X-GitHub-Event
X-GitHub-Delivery
X-Hub-Signature-256
```

Verify `X-Hub-Signature-256` as an HMAC-SHA256 signature over the unmodified raw body. Use `X-GitHub-Delivery` as the primary source delivery ID.

### 10.4 Grafana Alerting

Use Grafana HMAC signing with:

```text
X-Grafana-Alerting-Signature
X-Grafana-Alerting-Timestamp  // configured custom timestamp header
```

When a timestamp header is configured, verify:

```text
HMAC-SHA256(secret, timestamp + ":" + raw_body)
```

Require and validate timestamp freshness. Grafana payloads may contain multiple alert instances and therefore normalize to multiple internal events or one grouped event plus members, depending on the selected grouping policy.

### 10.5 Sentry

Use:

```text
Sentry-Hook-Signature
Sentry-Hook-Resource
```

Verify the Sentry signature against the original raw body with the configured integration client secret. Do not decode and re-encode JSON before verification.

### 10.6 Secret handling

- Secrets are referenced by environment variable name.
- Missing referenced secrets fail startup.
- Secret values are never returned by the status API.
- Secret values are never included in logs or GORM query arguments.
- Slack webhook URLs and Telegram bot tokens are secrets.
- SMTP passwords and API credentials are secrets.

---

## 11. Source Adapter Design

```go
type Source string

const (
    SourceWatcher Source = "watcher"
    SourceGitHub  Source = "github"
    SourceGrafana Source = "grafana"
    SourceSentry  Source = "sentry"
)

type InboundRequest struct {
    IntegrationID string
    Headers       http.Header
    RawBody       []byte
    ReceivedAt    time.Time
    RemoteIP      netip.Addr
}

type AdapterResult struct {
    SourceDeliveryID string
    SourceEventType  string
    Events           []Event
    IgnoreReason     string
}

type SourceAdapter interface {
    Source() Source
    Verify(ctx context.Context, integration Integration, req InboundRequest) error
    Normalize(ctx context.Context, integration Integration, req InboundRequest) (AdapterResult, error)
}
```

Verification and normalization are separate operations so an invalid request is rejected before expensive parsing and before untrusted payload details enter logs.

### Adapter requirements

Each adapter must:

1. Verify the source signature or configured authentication.
2. Extract a stable source delivery ID when available.
3. Recognize supported event types.
4. Return an ignored result for valid but intentionally unsupported events.
5. Decode payloads tolerantly.
6. Normalize timestamps to UTC.
7. Preserve unknown useful fields in metadata without making routing depend on raw provider structs.
8. Avoid placing secrets or full stack traces into normalized titles.
9. Produce deterministic fingerprints where the source does not provide one.

---

## 12. Source-Specific Event Ownership

Avoid duplicate notifications by defining which source is authoritative for each concern.

| Concern | Authoritative source |
|---|---|
| Deployment started, succeeded, failed, cancelled, rolled back | Watcher |
| Pull request and repository lifecycle | GitHub |
| Build and test workflow result | GitHub |
| Release publication | GitHub or Watcher, chosen explicitly per organization |
| Actual deployment result | Watcher |
| Backend and infrastructure health | Grafana |
| Frontend and application errors and regressions | Sentry |

GitHub deployment-related workflow completion should not be presented as the actual deployment result when Watcher owns deployment execution.

---

## 13. Watcher Contract

Because Watcher is controlled internally, define and version its webhook contract.

Recommended event types:

```text
watcher.version.found
watcher.deployment.started
watcher.deployment.succeeded
watcher.deployment.failed
watcher.deployment.cancelled
watcher.deployment.rolled_back
watcher.rollback.succeeded
watcher.rollback.failed
watcher.webhook.test
service.health.changed
webhook.delivery.exhausted
```

Recommended payload:

```json
{
  "schema_version": "v1",
  "event_id": "evt_01J...",
  "event_type": "watcher.deployment_failed",
  "occurred_at": "2026-06-18T08:42:10Z",
  "watcher": {
    "id": 12,
    "name": "api-prod"
  },
  "attempt": {
    "id": 302,
    "kind": "deploy",
    "reason": "new_version_found",
    "status": "failed",
    "triggered_by": "agent",
    "target_version": "v2.4.1",
    "from_version": "v2.4.0",
    "failure_phase": "health_check",
    "error": "health check returned 503",
    "parent_attempt_id": null,
    "root_attempt_id": 302
  },
  "summary": "Deployment of api-prod to v2.4.1 failed during health_check"
}
```

Watcher event ID is both the deployment identifier and source idempotency key when each lifecycle transition has a distinct event ID. If the same deployment ID is reused across transitions, combine it with the event name and occurrence timestamp.

---

## 14. GitHub Adapter Scope

Initial supported event families:

```text
pull_request
workflow_run
release
```

Optional later families:

```text
security_advisory
code_scanning_alert
dependabot_alert
repository_vulnerability_alert
```

Recommended normalized event types:

```text
github.pull_request.opened
github.pull_request.merged
github.pull_request.closed
github.workflow.succeeded
github.workflow.failed
github.workflow.cancelled
github.release.published
```

Do not route every `push` event by default. High-volume repository activity should be explicitly opted into.

Use:

```text
source_delivery_id = X-GitHub-Delivery
source_event_type  = X-GitHub-Event
```

Correlation fields:

```text
repository
branch
commit_sha
workflow_name
run_id
pull_request_number
release_tag
actor
```

---

## 15. Grafana Adapter Scope

Grafana payloads can contain a group with multiple alerts. Preserve:

```text
groupKey
receiver
status
commonLabels
commonAnnotations
alerts[].fingerprint
alerts[].status
alerts[].labels
alerts[].annotations
alerts[].startsAt
alerts[].endsAt
alerts[].generatorURL
alerts[].dashboardURL
alerts[].panelURL
```

Recommended normalization strategy for version 1:

- Create one internal event per `alerts[]` item.
- Copy the Grafana `groupKey` into each event.
- Use the alert `fingerprint` as the source fingerprint.
- Keep a shared `receipt_id` across the group.
- Let renderers optionally group multiple delivery jobs from the same receipt in a later version.

Normalized event types:

```text
grafana.alert.firing
grafana.alert.resolved
```

Expected labels:

```text
alertname
service
environment
team
severity
region
component
```

When Grafana does not provide a stable delivery ID, derive the receipt idempotency key from a canonical combination of integration ID, timestamp header when available, signature, and raw-body hash.

---

## 16. Sentry Adapter Scope

Initial supported resources and actions should focus on signal rather than every event occurrence:

```text
issue alerts
issue created
issue regression
issue resolved
metric alert triggered
metric alert resolved
```

Recommended normalized event types:

```text
sentry.issue.triggered
sentry.issue.regressed
sentry.issue.resolved
sentry.metric.triggered
sentry.metric.resolved
```

Extract when available:

```text
project
organization
issue_id
issue_short_id
title
culprit
level
environment
release
first_seen
last_seen
event_count
browser
device
issue_url
```

Do not notify on every raw error occurrence. Sentry alert-rule and lifecycle notifications should be the primary ingress signal.

If no stable delivery ID is provided, derive one from stable resource identifiers, action, occurrence timestamp, and raw-body hash.

---

## 17. Normalized Event Model

```go
type Lifecycle string

type Severity string

type Event struct {
    ID              string
    ReceiptID       string
    Source          Source
    IntegrationID   string
    SourceEventID   string
    Type            string
    Action          string
    Lifecycle       Lifecycle
    Severity        Severity

    Title           string
    Summary         string
    Service         string
    Environment     string
    Repository      string
    Branch          string
    Release         string
    CommitSHA       string
    Actor           string

    Fingerprint     string
    GroupKey        string
    URL             string

    OccurredAt      time.Time
    StartedAt       *time.Time
    EndedAt         *time.Time

    Labels          map[string]string
    Fields          map[string]any
}
```

### Lifecycle values

```text
started
triggered
updated
succeeded
failed
resolved
cancelled
rolled_back
```

### Severity values

```text
debug
info
warning
error
critical
```

Source-provided severities must be mapped explicitly. Unknown values map to a configured default, usually `warning`, while preserving the original value in `fields`.

### Event type naming

Use:

```text
<source>.<domain>.<action>
```

Examples:

```text
watcher.deployment.failed
github.workflow.failed
grafana.alert.firing
sentry.issue.regressed
```

---

## 18. Idempotency and Correlation

### 18.1 Receipt idempotency

Primary uniqueness:

```text
(source, integration_id, source_delivery_id)
```

For sources without a stable delivery ID, use a deterministic derived ID. The raw payload SHA-256 hash should always be stored for troubleshooting and fallback deduplication.

### 18.2 Delivery idempotency

Prevent duplicate route matches from creating duplicate destination jobs:

```text
(event_id, destination_id)
```

If the same event reaches the same destination through multiple route policies, only one delivery row is created. Store all matched route IDs as metadata or a join table when audit detail is needed.

### 18.3 Correlation

Cross-source correlation is advisory, not a hard deduplication rule. Recommended keys:

```text
service
environment
commit_sha
release
deployment_id
repository
```

Do not automatically suppress a Grafana alert merely because a Watcher deployment event exists. They represent different facts.

---

## 19. Routing Policy Model

Routing is database-backed and additive. Operators manage route records through the operational API, and the in-memory routing engine reloads from persisted route state after each mutation. All matching routes contribute destinations. Duplicated destination IDs are collapsed before delivery rows are inserted.

Matching semantics:

- Different match fields use logical AND.
- Multiple values within one field use logical OR.
- Empty match fields mean unrestricted.
- Selector arrays must not contain blank strings; omit a field instead of sending `[""]` when you want a wildcard.
- Label matches require exact values in version 1.
- Route order affects audit presentation, not delivery semantics.

Example:

```yaml
routes:
  - id: production-critical
    match:
      environments: [production]
      severities: [critical]
    destinations:
      - telegram-bot
      - slack-operations
      - email-operations

  - id: deployment-failed
    match:
      types:
        - watcher.deployment.failed
    destinations:
      - telegram-bot
      - slack-deployments
      - email-operations

  - id: deployment-succeeded
    match:
      types:
        - watcher.deployment.succeeded
    destinations:
      - slack-deployments

  - id: backend-alerts
    match:
      sources: [grafana]
      environments: [production]
      severities: [warning, error, critical]
    destinations:
      - slack-operations

  - id: frontend-regression
    match:
      types:
        - sentry.issue.regressed
      environments: [production]
    destinations:
      - slack-frontend
      - email-frontend
```

### Unrouted events

A valid event with no matching destinations remains persisted. Mark it as `unrouted` in event metadata and expose it in the operational API.

---

## 20. Destination Model

```go
type DestinationType string

const (
    DestinationSlack    DestinationType = "slack"
    DestinationTelegram DestinationType = "telegram"
    DestinationEmail    DestinationType = "email"
)

type Destination struct {
    ID       string
    Type     DestinationType
    Enabled  bool
    Profile  string
    Config   map[string]any
}
```

Destination configuration is decoded into provider-specific typed configuration during startup. The untyped map does not flow into senders.

### Slack destination

```yaml
slack-deployments:
  type: slack
  webhook_url_env: SLACK_DEPLOYMENTS_WEBHOOK_URL
  profile: detailed
```

An incoming webhook URL is channel-specific. Create separate named destinations for separate Slack channels.

### Telegram destination

```yaml
telegram-bot:
  type: telegram
  bot_token_env: TELEGRAM_ONCALL_BOT_TOKEN
  chat_id: "-100123456789"
  profile: compact
```

### Email destination

```yaml
email-operations:
  type: email
  smtp: primary
  from: alerts@example.com
  to:
    - ops@example.com
    - oncall@example.com
  profile: detailed
```

Recipient groups may be introduced if recipient lists are repeated across destinations.

---

## 21. Message Rendering

Use a destination-neutral render input and destination-specific output.

```go
type MessageModel struct {
    Subject     string
    Title       string
    Summary     string
    Severity    Severity
    Lifecycle   Lifecycle
    Fields      []MessageField
    Actions     []MessageAction
    Footer      string
    OccurredAt  time.Time
}

type Renderer interface {
    Render(ctx context.Context, event Event, destination Destination) (RenderedMessage, error)
}
```

Version 1 renderers are code-defined. Avoid arbitrary runtime templates initially because they add runtime failures, escaping problems, and a configuration language that must be supported indefinitely.

### Slack

- Use Block Kit-compatible JSON sent through an incoming webhook.
- Include a compact heading, severity, environment, important fields, and source URL.
- Escape provider-controlled content.
- Do not attempt to override channel, icon, or username dynamically.

### Telegram

- Use compact messages suitable for mobile alerts.
- Prefer HTML parse mode with explicit HTML escaping, or plain text.
- Include one primary link button when practical.
- Truncate long summaries and errors.

### Email

- Produce a meaningful subject line.
- Generate both text and HTML bodies.
- Validate recipient addresses during startup.
- Prevent newline injection into subject and address headers.
- Use embedded templates compiled at startup.

Example subject:

```text
[PRODUCTION][CRITICAL] Deployment failed: auth-service v2.4.1
```

---

## 22. Sender Interfaces

```go
type SendResult struct {
    ProviderMessageID string
    ResponseCode      int
    RetryAfter        time.Duration
}

type SendError struct {
    Retryable    bool
    Code         string
    ResponseCode int
    Err          error
}

type Sender interface {
    Type() DestinationType
    Send(ctx context.Context, destination Destination, message RenderedMessage) (SendResult, error)
}
```

Each provider adapter classifies failures rather than forcing the delivery service to understand provider-specific responses.

### Retryable examples

- Network timeout.
- Temporary DNS failure.
- HTTP `408`.
- HTTP `429`, honoring `Retry-After`.
- HTTP `5xx`.
- Temporary SMTP response.

### Permanent examples

- Invalid Slack webhook URL or revoked webhook.
- Telegram chat not found.
- Invalid bot token.
- Invalid email address.
- Permanent SMTP rejection.
- Payload rejected because the gateway generated an invalid provider message.

Permanent errors move directly to `dead_letter` unless configuration explicitly permits retries.

---

## 23. Persistence Model

Use separate domain types and GORM persistence models.

## 23.1 `webhook_receipts`

| Column | Purpose |
|---|---|
| `id` | Gateway receipt ULID |
| `source` | watcher, github, grafana, sentry |
| `integration_id` | Configured integration |
| `source_delivery_id` | Provider or derived idempotency key |
| `source_event_type` | Provider event type or resource |
| `payload_sha256` | Raw-body digest |
| `raw_payload` | Optional retained payload bytes |
| `headers_json` | Sanitized selected headers only |
| `received_at` | Gateway receive time |
| `status` | accepted, ignored, duplicate, failed, unrouted |
| `ignore_reason` | Optional reason |
| `created_at` | Persistence time |

Unique index:

```text
(source, integration_id, source_delivery_id)
```

Never store authorization headers, signatures, bot tokens, webhook URLs, or cookies in `headers_json`.

## 23.2 `events`

| Column | Purpose |
|---|---|
| `id` | Event ULID |
| `receipt_id` | Parent receipt |
| `source_event_id` | Source-specific event or resource ID |
| `type` | Normalized type |
| `action` | Source action |
| `lifecycle` | Common lifecycle |
| `severity` | Common severity |
| `title` | Render-safe title |
| `summary` | Render-safe summary |
| `service` | Service identity |
| `environment` | Runtime environment |
| `repository` | Repository identity |
| `branch` | Branch |
| `release` | Release/version |
| `commit_sha` | Commit |
| `actor` | Initiating actor |
| `fingerprint` | Alert correlation |
| `group_key` | Source grouping |
| `url` | Source details URL |
| `occurred_at` | Source occurrence time |
| `started_at` | Optional |
| `ended_at` | Optional |
| `labels_json` | Normalized labels |
| `fields_json` | Additional normalized fields |
| `created_at` | Persistence time |

Indexes:

```text
receipt_id
type, occurred_at
source, occurred_at
service, environment, occurred_at
fingerprint, occurred_at
```

## 23.3 `deliveries`

| Column | Purpose |
|---|---|
| `id` | Delivery ULID |
| `event_id` | Parent normalized event |
| `destination_id` | Configured destination |
| `destination_type` | slack, telegram, email |
| `status` | Delivery state |
| `attempt_count` | Completed send attempts |
| `max_attempts` | Retry limit captured at creation |
| `next_attempt_at` | Next eligible attempt |
| `locked_by` | Worker identity |
| `locked_until` | Recovery lease |
| `provider_message_id` | Optional provider result |
| `last_error_code` | Stable classification |
| `last_error` | Sanitized error summary |
| `sent_at` | Successful send time |
| `created_at` | Creation time |
| `updated_at` | Last transition |

Unique index:

```text
(event_id, destination_id)
```

Poll index:

```text
(status, next_attempt_at)
```

## 23.4 `delivery_attempts`

| Column | Purpose |
|---|---|
| `id` | Attempt ULID |
| `delivery_id` | Delivery |
| `attempt_number` | Attempt sequence |
| `worker_id` | Worker |
| `started_at` | Start |
| `completed_at` | Completion |
| `outcome` | sent, retryable_failure, permanent_failure |
| `response_code` | HTTP or provider response when applicable |
| `error_code` | Stable classification |
| `error_message` | Sanitized summary |
| `duration_ms` | Send latency |

Unique index:

```text
(delivery_id, attempt_number)
```

## 23.5 `schema_migrations`

Track applied embedded migrations explicitly.

---

## 24. Ingestion Transaction

The ingestion service performs:

```text
1. Resolve integration from path.
2. Ensure path source matches integration source.
3. Enforce request limits.
4. Verify signature against raw bytes.
5. Normalize the source payload.
6. Derive source delivery ID.
7. Evaluate routes for each normalized event.
8. Begin SQLite transaction.
9. Insert receipt using unique source delivery constraint.
10. If duplicate, roll back or return existing receipt as duplicate.
11. Insert normalized events.
12. Insert one delivery per unique destination.
13. Mark receipt accepted, ignored, or unrouted.
14. Commit.
15. Return 202.
```

Routing occurs before the transaction because it is deterministic and in-memory. The transaction contains only database operations.

If any event or delivery insert fails, the whole receipt batch rolls back. A Grafana group is therefore accepted atomically.

---

## 25. Delivery State Machine

```text
pending ───────────────► processing ───────────────► sent
   ▲                         │
   │                         ├── retryable failure ─► retry_wait
   │                         │                           │
   │                         ├── permanent failure ─► dead_letter
   │                         │
   └──── expired lease ◄─────┘

retry_wait ── due ──► processing
```

States:

```text
pending
processing
retry_wait
sent
dead_letter
cancelled
```

A manual retry changes `dead_letter` or eligible failed state to `retry_wait` with `next_attempt_at = now`, without deleting attempt history.

---

## 26. Worker and Claiming Design

SQLite does not provide a PostgreSQL-style `SKIP LOCKED` workflow. Version 1 uses one scheduler goroutine per process and a configurable delivery worker pool.

### Scheduler

1. Poll due jobs at a configured interval.
2. Open a short transaction.
3. Select a limited batch where:
   - status is `pending` or `retry_wait`, and
   - `next_attempt_at <= now`, and
   - no active lease exists.
4. Update selected jobs to `processing` with `locked_by` and `locked_until`.
5. Commit.
6. Send claimed IDs to the in-process worker channel.

### Worker

1. Load delivery, event, and destination configuration.
2. Render outside a transaction.
3. Send outside a transaction.
4. Open a short completion transaction.
5. Insert attempt history.
6. Mark delivery `sent`, `retry_wait`, or `dead_letter`.
7. Commit.

### Lease recovery

A housekeeping loop periodically moves expired `processing` deliveries back to `retry_wait`. This recovers jobs claimed immediately before a process crash.

### Important limitation

If the external provider accepted a message and the process crashed before persisting `sent`, the message can be delivered twice. This is the expected at-least-once behavior.

---

## 27. Retry Policy

Recommended defaults:

```yaml
retry:
  max_attempts: 8
  base_delay: 2s
  max_delay: 15m
  jitter: 0.20
```

Backoff:

```text
min(max_delay, base_delay * 2^(attempt-1)) + jitter
```

Provider `Retry-After` must take precedence when it is longer than the computed delay and remains within an operationally acceptable bound.

Do not retry configuration errors forever. Dead-letter permanent failures with a stable reason code.

---

## 28. SQLite Configuration

Recommended startup behavior:

```text
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;
```

Recommended Go database settings for version 1:

```go
sqlDB.SetMaxOpenConns(1)
sqlDB.SetMaxIdleConns(1)
sqlDB.SetConnMaxLifetime(0)
```

The database file must live on a persistent local filesystem. Do not run multiple gateway replicas against one SQLite file on a shared volume.

### Migration trigger

Move to PostgreSQL or another server database when any of these becomes necessary:

- Multiple active gateway replicas.
- High availability without active/passive ownership.
- Sustained write contention.
- Remote database access from separate worker processes.
- Large retention and reporting requirements.
- Operational teams need database-native replication or point-in-time recovery.

Repository interfaces should make this migration possible without changing source or destination adapters.

---

## 29. Configuration Design

Example configuration:

```yaml
server:
  address: ":8080"
  read_header_timeout: 5s
  read_timeout: 15s
  write_timeout: 15s
  idle_timeout: 60s
  shutdown_timeout: 20s
  max_webhook_body_bytes: 1048576
  trusted_proxies: []

api:
  admin_token_env: GATEWAY_ADMIN_TOKEN
  docs_enabled: true

logging:
  level: info
  format: json
  add_source: false

database:
  path: ./data/gateway.db
  busy_timeout: 5s
  max_open_connections: 1
  retain_raw_payloads: true
  raw_payload_retention: 14d
  event_retention: 90d
  attempt_retention: 90d

workers:
  poll_interval: 1s
  batch_size: 50
  concurrency: 8
  lease_duration: 2m
  recovery_interval: 30s

retry:
  max_attempts: 8
  base_delay: 2s
  max_delay: 15m
  jitter: 0.20

integrations:
  watcher-production:
    source: watcher
    secret_env: WATCHER_WEBHOOK_SECRET
    replay_window: 5m

  github-main:
    source: github
    secret_env: GITHUB_WEBHOOK_SECRET

  grafana-production:
    source: grafana
    secret_env: GRAFANA_WEBHOOK_SECRET
    signature_header: X-Grafana-Alerting-Signature
    timestamp_header: X-Grafana-Alerting-Timestamp
    replay_window: 5m

  sentry-frontend:
    source: sentry
    client_secret_env: SENTRY_INTEGRATION_CLIENT_SECRET

smtp:
  primary:
    host: smtp.example.com
    port: 587
    username_env: SMTP_USERNAME
    password_env: SMTP_PASSWORD
    starttls: required
    connect_timeout: 5s
    send_timeout: 15s


destinations:
  slack-operations:
    type: slack
    webhook_url_env: SLACK_OPERATIONS_WEBHOOK_URL
    profile: detailed

  slack-deployments:
    type: slack
    webhook_url_env: SLACK_DEPLOYMENTS_WEBHOOK_URL
    profile: detailed

  telegram-bot:
    type: telegram
    bot_token_env: TELEGRAM_ONCALL_BOT_TOKEN
    chat_id: "-100123456789"
    profile: compact

  email-operations:
    type: email
    smtp: primary
    from: alerts@example.com
    to:
      - ops@example.com
      - oncall@example.com
    profile: detailed
```

### Configuration validation

Startup must fail when:

- Integration IDs are duplicated.
- Destination IDs are duplicated.
- A required secret environment variable is absent.
- A source or destination type is unsupported.
- A replay window is invalid.
- An SMTP reference is unknown.
- An email address is invalid.
- Worker, retry, or timeout values are out of bounds.
- SQLite path is empty or its parent directory cannot be created.

Viper is an input mechanism. After loading, unmarshal into typed configuration structs, resolve secret references, validate, and pass an immutable config object to the application. Route validation happens in the route CRUD service because routes are no longer startup-loaded config.

---

## 30. Logging and Observability

### 30.1 Contextual logger

```go
package logctx

func With(ctx context.Context, attrs ...any) context.Context
func From(ctx context.Context) *slog.Logger
```

HTTP middleware creates:

```text
request_id
method
path
remote_ip
user_agent
```

Every inbound HTTP request should emit one `http.request` access log after the request finishes, even if a later middleware or handler rejects it. Webhook ingestion should emit a second outcome log such as `webhook.accepted`, `webhook.ignored`, `webhook.unrouted`, or `webhook.duplicate` so operators can distinguish transport acceptance from delivery fan-out.

The ingestion service adds:

```text
source
integration_id
receipt_id
source_delivery_id
```

Delivery workers add:

```text
delivery_id
event_id
destination_id
attempt
worker_id
```

### 30.2 Log events

Recommended stable messages:

```text
http.request
webhook.received
webhook.rejected
webhook.duplicate
webhook.accepted
webhook.ignored
webhook.unrouted
delivery.claimed
delivery.sent
delivery.retry_scheduled
delivery.dead_lettered
delivery.lease_recovered
config.loaded
migration.applied
server.started
server.stopping
```

### 30.3 Redaction

Never log:

- Raw authorization headers.
- Signature values.
- Slack webhook URLs.
- Telegram bot tokens.
- SMTP passwords.
- Full raw payloads by default.
- Cookies.

GORM logging should use a slog-compatible adapter, avoid query argument logging in production, and log slow queries at warning level.

### 30.4 Metrics

A metrics endpoint may be added later. Useful counters and gauges:

```text
webhooks_received_total
webhooks_rejected_total
webhooks_duplicate_total
events_normalized_total
events_unrouted_total
deliveries_created_total
deliveries_sent_total
deliveries_failed_total
deliveries_dead_letter_total
delivery_attempt_duration_seconds
delivery_queue_depth
oldest_pending_delivery_age_seconds
```

---

## 31. Project Structure

```text
cmd/
  gateway/
    main.go

internal/
  app/
    app.go
    lifecycle.go

  config/
    config.go
    load.go
    validate.go
    secrets.go

  domain/
    event.go
    receipt.go
    delivery.go
    destination.go
    errors.go

  httpserver/
    server.go
    router.go
    huma.go

    middleware/
      recovery.go
      request_id.go
      access_log.go
      body_limit.go
      admin_auth.go

    webhook/
      routes.go
      watcher.go
      github.go
      grafana.go
      sentry.go
      models.go

    admin/
      routes.go
      events.go
      deliveries.go
      status.go

    health/
      routes.go

  ingress/
    service.go
    adapter.go
    registry.go
    request.go

    watcher/
      adapter.go
      payload.go
      verify.go
      normalize.go

    github/
      adapter.go
      payload.go
      verify.go
      normalize.go

    grafana/
      adapter.go
      payload.go
      verify.go
      normalize.go

    sentry/
      adapter.go
      payload.go
      verify.go
      normalize.go

  routing/
    engine.go
    policy.go
    matcher.go

  message/
    model.go
    profiles.go

    slack/
      renderer.go

    telegram/
      renderer.go

    email/
      renderer.go
      templates.go

  delivery/
    service.go
    scheduler.go
    worker.go
    retry.go
    classifier.go

  sender/
    sender.go
    registry.go

    slack/
      sender.go
      payload.go

    telegram/
      sender.go
      payload.go

    email/
      sender.go
      smtp.go
      message.go

  storage/
    store.go
    transaction.go
    receipt_repository.go
    event_repository.go
    delivery_repository.go

    sqlite/
      open.go
      migrate.go
      models.go
      receipt_repository.go
      event_repository.go
      delivery_repository.go
      transaction.go
      migrations/
        001_initial.sql
        002_indexes.sql

  observability/
    logging.go
    logctx/
      context.go

  clock/
    clock.go

  id/
    id.go

configs/
  config.example.yaml

templates/
  email/
    alert.txt.tmpl
    alert.html.tmpl

testdata/
  watcher/
  github/
  grafana/
  sentry/

docs/
  design.md
  integrations/
    watcher.md
    github.md
    grafana.md
    sentry.md

Dockerfile
compose.yaml
go.mod
go.sum
README.md
```

The package tree is a target shape, not a requirement to create every file before implementing the first vertical slice.

---

## 32. Core Interfaces

### Store

```go
type Store interface {
    Ingest(ctx context.Context, batch IngestBatch) (IngestResult, error)
    GetEvent(ctx context.Context, id string) (Event, error)
    ListEvents(ctx context.Context, filter EventFilter) (EventPage, error)
    GetDelivery(ctx context.Context, id string) (Delivery, error)
    ListDeliveries(ctx context.Context, filter DeliveryFilter) (DeliveryPage, error)
    ClaimDueDeliveries(ctx context.Context, claim ClaimRequest) ([]Delivery, error)
    CompleteAttempt(ctx context.Context, result AttemptResult) error
    RecoverExpiredLeases(ctx context.Context, now time.Time) (int64, error)
    RetryDelivery(ctx context.Context, id string, now time.Time) error
    ApplyRetention(ctx context.Context, now time.Time, policy RetentionPolicy) error
    Ping(ctx context.Context) error
    Close() error
}
```

The atomic `Ingest` method owns the transaction that inserts receipt, events, and deliveries. Avoid exposing transaction mechanics to source adapters.

### Router

```go
type Router interface {
    Destinations(event Event) []RouteMatch
}
```

### Renderer registry

```go
type RendererRegistry interface {
    RendererFor(destination Destination) (Renderer, error)
}
```

### Sender registry

```go
type SenderRegistry interface {
    SenderFor(destination Destination) (Sender, error)
}
```

### Clock

Use an injectable clock for deterministic retry, lease, replay-window, and retention tests.

```go
type Clock interface {
    Now() time.Time
}
```

---

## 33. Application Bootstrap

Conceptual wiring:

```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    cfg := config.MustLoad()
    logger := observability.NewLogger(cfg.Logging)

    store := sqlite.MustOpen(cfg.Database, logger)
    adapters := ingress.NewRegistry(
        watcher.NewAdapter(),
        github.NewAdapter(),
        grafana.NewAdapter(),
        sentry.NewAdapter(),
    )

    routeEngine := routing.NewEngine(nil)
    destinations := sender.NewDestinationRegistry(cfg.Destinations)
    renderers := message.NewRendererRegistry()
    senders := sender.NewRegistry(cfg, logger)

    ingestService := ingress.NewService(store, adapters, routeEngine, clock.Real{}, logger)
    appService := app.NewService(cfg, clock.Real{}, store, ingestService, routeEngine)
    if err := appService.LoadRoutes(ctx); err != nil {
        logger.Error("routes.load_failed", "error", err)
        os.Exit(1)
    }
    deliveryService := delivery.NewService(store, destinations, renderers, senders, cfg.Workers, cfg.Retry, clock.Real{}, logger)

    router := gin.New()
    api := humagin.New(router, huma.DefaultConfig("Webhook Notification Gateway", "1.0.0"))

    httpserver.RegisterMiddleware(router, cfg, logger)
    httpserver.RegisterWebhookRoutes(api, ingestService)
    httpserver.RegisterAdminRoutes(api, store, deliveryService)
    httpserver.RegisterHealthRoutes(api, store, deliveryService)

    app := app.New(router, deliveryService, store, cfg.Server, logger)
    if err := app.Run(ctx); err != nil {
        logger.Error("application stopped", "error", err)
        os.Exit(1)
    }
}
```

Constructors should return errors rather than panic in library packages. A `Must...` helper is acceptable only at the outermost startup boundary.

---

## 34. Graceful Shutdown

On shutdown:

1. Stop accepting new HTTP requests.
2. Stop the delivery scheduler from claiming new work.
3. Allow in-flight HTTP requests to complete within the server shutdown timeout.
4. Allow in-flight delivery workers to finish within the worker shutdown timeout.
5. Do not begin new provider sends after cancellation.
6. Jobs left in `processing` recover through lease expiry after restart.
7. Close the database last.

Do not mark a delivery failed solely because application shutdown cancelled its context. Leave it recoverable through lease expiry unless a provider result is known.

---

## 35. Testing Strategy

## 35.1 Unit tests

- HMAC verification with known test vectors.
- Replay-window validation.
- Source payload normalization from fixtures.
- Severity mapping.
- Route matching tables.
- Duplicate destination collapse.
- Retry classification and backoff.
- Slack, Telegram, and email renderer golden files.
- Escaping and truncation.
- Config validation.

## 35.2 Repository integration tests

Use a temporary SQLite database for each test or test group.

Cover:

- Migrations from empty database.
- Receipt uniqueness.
- Atomic receipt/event/delivery insertion.
- Claiming due jobs.
- Lease expiry recovery.
- Delivery completion transitions.
- Manual retry.
- Retention behavior.
- Foreign-key enforcement.

## 35.3 HTTP tests

Use `httptest` with the Gin/Huma application wiring.

Cover:

- Correct signature accepted.
- Incorrect signature rejected.
- Raw-body verification remains valid.
- Body limit.
- Duplicate webhook response.
- Ignored event response.
- Persistence failure returns non-2xx.
- Admin authentication.
- OpenAPI document generation.

## 35.4 Worker tests

Use fake senders and a fake clock.

Cover:

- Successful delivery.
- Temporary failure then success.
- Provider `Retry-After`.
- Permanent failure to dead letter.
- Crash-like lease expiry.
- Multiple destinations fail independently.
- Context cancellation during shutdown.

## 35.5 End-to-end test

A complete test should:

1. Start the gateway with a temporary SQLite database.
2. Configure a Watcher integration and fake destination server.
3. Send a signed deployment webhook.
4. Assert `202`.
5. Wait for the fake provider request.
6. Assert the delivery and attempt rows are `sent`.
7. Send the same webhook again.
8. Assert no second provider message is sent.

---

## 36. Implementation Phases

## Phase 0 — Repository foundation

Deliverables:

- Go module.
- Configuration structs and validation.
- Slog initialization and contextual logger helper.
- Gin plus HumaGin bootstrap.
- Health endpoints.
- Graceful shutdown.
- SQLite open, pragmas, embedded migration runner.

Acceptance criteria:

- Service starts with valid config.
- Invalid config fails with actionable errors.
- `/docs`, `/openapi.json`, `/health/live`, and `/health/ready` work.
- SQLite migrations are repeatable.

## Phase 1 — Durable Watcher-to-Slack vertical slice

Deliverables:

- Watcher signed contract.
- Normalized event model.
- Routing engine.
- Receipt/event/delivery persistence.
- Slack renderer and sender.
- Scheduler, worker, retries, attempt history.

Acceptance criteria:

- A valid Watcher deployment failure becomes one Slack message.
- The same Watcher event is idempotent.
- Slack `5xx` retries.
- Slack permanent failure dead-letters.
- Restart preserves pending work.

## Phase 2 — Telegram and email

Deliverables:

- Telegram renderer and sender.
- Email text/HTML renderer.
- SMTP sender.
- Destination-specific retry classification.

Acceptance criteria:

- One event can fan out independently to all three destination types.
- Failure in one destination does not block the others.
- Secrets are absent from logs and operational API output.

## Phase 3 — GitHub adapter

Deliverables:

- Signature verification.
- `pull_request`, `workflow_run`, and `release` fixtures.
- Normalization and routing examples.

Acceptance criteria:

- GitHub documented signature test vector passes.
- `X-GitHub-Delivery` provides idempotency.
- Unsupported valid events return `202` as ignored.

## Phase 4 — Grafana adapter

Deliverables:

- HMAC and timestamp verification.
- Group payload parsing.
- One normalized event per alert instance.
- Firing and resolved lifecycle rendering.

Acceptance criteria:

- Multi-alert receipt is atomic.
- Fingerprints and group keys are preserved.
- Replay timestamps outside the window are rejected.

## Phase 5 — Sentry adapter

Deliverables:

- Signature verification.
- Issue and metric alert fixtures.
- Triggered, regressed, and resolved normalization.

Acceptance criteria:

- Raw-body signature verification is covered by tests.
- Regressions route distinctly from first occurrence.
- High-volume raw occurrence events are ignored unless explicitly enabled.

## Phase 6 — Operational API and hardening

Deliverables:

- Event and delivery list/detail APIs.
- Manual retry.
- Retention worker.
- Request and provider timeout tuning.
- Container image and example deployment.
- Backup and restore documentation.

Acceptance criteria:

- Failed deliveries can be diagnosed and retried.
- Retention removes old raw payloads without breaking current delivery records.
- Shutdown and restart tests pass.

---

## 37. Security Checklist

- [ ] HTTPS enforced at deployment boundary.
- [ ] All source integrations require signatures or equivalent authentication.
- [ ] Raw bytes verified before JSON transformation.
- [ ] Constant-time comparison used.
- [ ] Replay timestamps checked where supported.
- [ ] Webhook body-size limit enabled.
- [ ] Admin API protected.
- [ ] Secrets referenced through environment variables.
- [ ] Secrets and signatures redacted from logs.
- [ ] Outbound URLs come only from validated configuration.
- [ ] Redirect following disabled or restricted for provider clients.
- [ ] Provider content escaped during rendering.
- [ ] Email headers protected from newline injection.
- [ ] Raw payload retention is configurable and documented.
- [ ] SQLite file permissions are restricted.
- [ ] Backups are encrypted or stored in a protected location.

---

## 38. Operational Runbook Notes

### Backup

Use a SQLite-safe backup approach. Do not copy the database file blindly while it is actively being written unless the backup method correctly accounts for WAL state.

### Disk monitoring

Alert on:

- Database file growth.
- WAL file growth.
- Filesystem free space.
- Oldest pending delivery age.
- Dead-letter count.

### Provider outage

The gateway remains ready and continues accepting webhooks while a destination provider is unavailable, as long as SQLite remains writable. Queue growth must be monitored.

### Database corruption or unavailable disk

Readiness fails and webhook ingestion returns `503`, causing capable sources to retry. Do not return success when durable storage failed.

### Secret rotation

Version 1 rotates secrets through environment/config change and process restart. Support for overlapping old and new ingress secrets may be added later by allowing a list of active secret references per integration.

---

## 39. Explicit Design Decisions

### ADR-001: Single binary with in-process workers

Chosen for operational simplicity and because expected webhook volume is modest. The durable store and interfaces preserve a future split.

### ADR-002: SQLite durable inbox/outbox

Chosen to avoid external infrastructure while still surviving restarts and supporting retries. This constrains deployment to one active instance with local persistent storage.

### ADR-003: Huma operations for webhooks

Chosen because Huma supports raw request bytes. This allows accurate signature verification without sacrificing OpenAPI documentation.

### ADR-004: Startup-defined integrations plus persisted live routes (SUPERCEDED BY ADR-008)

Chosen to keep credentials and transport settings deterministic at process start while still allowing operators to adjust routing without a restart. Integrations and destinations remain config-owned; route policy state is persisted in SQLite and edited through CRUD admin API endpoints.

### ADR-005: Code-defined renderers

Chosen to guarantee escaping, validation, and tests. Arbitrary templates are deferred until there is a clear operational need.

### ADR-006: Additive routing with destination deduplication

Chosen so multiple independent policies can match one event without causing duplicate delivery to the same named destination.

### ADR-007: At-least-once outbound semantics

Chosen because no atomic transaction spans SQLite and external providers. The system documents the possible crash-window duplicate rather than claiming exactly-once behavior.

### ADR-008: Dynamic Integrations and Destinations with SQLite Encryption

Supercedes ADR-004. Chosen to allow operators to configure webhook integration endpoints (Watcher, GitHub, Grafana, Sentry) and delivery destinations (Slack, Telegram, Email) dynamically via REST API endpoints without restarting the gateway process. Provider secrets, such as Slack webhooks and Telegram bot tokens, are encrypted in SQLite using AES-256-GCM. A master encryption key (`GATEWAY_ENCRYPTION_KEY`) must be supplied at startup. Hot-reloaded in-memory registries cache the configurations to keep webhook ingestion and background delivery performance high and avoid continuous SQLite lookups.

---

## 40. Definition of Done for Version 1

Version 1 is complete when:

1. Watcher, GitHub, Grafana, and Sentry webhook signatures are verified.
2. Supported source events normalize into a documented common model.
3. Routes can target Slack, Telegram, and email.
4. Accepted work is durable before `202` is returned.
5. Source retries do not duplicate work when idempotency information is available.
6. Temporary destination failures retry with bounded exponential backoff.
7. Permanent failures become inspectable dead letters.
8. Restart recovers queued and leased work.
9. Operational APIs show events, deliveries, and attempts.
10. Operators can manually retry dead-lettered deliveries.
11. OpenAPI documentation is generated through HumaGin.
12. Logs are structured, correlated, and free from secrets.
13. Integration, worker, repository, and end-to-end tests cover the critical flows.
14. Deployment documentation states the single-instance SQLite constraint clearly.

---

## 41. Recommended First Coding Task

Implement the smallest durable vertical slice:

```text
signed Watcher deployment_failed webhook
    ↓
Huma RawBody verification
    ↓
normalize Watcher event
    ↓
match one route
    ↓
transactionally persist receipt + event + Slack delivery
    ↓
return 202
    ↓
worker sends Slack message
    ↓
persist attempt and sent status
```

This slice validates every important architectural boundary before adding more sources and destinations.
