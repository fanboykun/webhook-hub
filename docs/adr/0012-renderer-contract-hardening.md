# ADR 0012: Renderer Contract Hardening Before First Release

## Decision

The gateway enforces event payload definitions before persistence and compiles renderer templates before publishing dynamic configuration.

- Watcher events expose event-specific schemas rather than one union payload.
- Payload versions, required fields, declared JSON types, and unknown fields are validated at ingress.
- Missing optional fields are materialized with deterministic zero values for template execution.
- Templates use a restricted syntax and declared scalar fields only.
- Provider-controlled values are escaped before execution and rendered output limits are enforced.
- Optional source links must be absolute HTTP(S) URLs before events are persisted.
- Dynamic configuration seeding, including routes, is transactional; CRUD and route changes are serialized and validated before registry publication.
- Destinations and renderer profiles cannot be deleted while referenced by routes, active deliveries, or destinations.
- `workers.concurrency` starts that many independent delivery schedulers.

## Migration

Migration `009_reset_unshipped_renderer_contract.sql` deletes existing receipts, events, deliveries, routes, destinations, and renderer profiles. This is intentionally destructive because the application has not shipped and the old persisted renderer JSON and destination profile shape are incompatible with the corrected contract. Integrations remain because their persisted shape did not change.

## Consequences

- Invalid source payloads fail before any receipt or event is persisted.
- Invalid templates or dynamic references never replace a valid live registry snapshot.
- Operators must configure secret fields with environment variable names, not inline secret values.
- Existing development databases lose notification history, destinations, renderer profiles, and routes once migration 009 runs; configured defaults repopulate empty tables on the next startup.
