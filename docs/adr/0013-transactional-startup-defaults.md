# ADR 0013: Transactional Startup Defaults

## Decision

File-configured integrations, destinations, renderer profiles, and routes are typed startup defaults. Bootstrap inserts defaults only when the corresponding SQLite table is empty, and seeds all four tables in one transaction.

After seeding, SQLite is authoritative. Restarting does not reconcile or overwrite a non-empty table. Bootstrap loads and validates complete persisted snapshots, then publishes the dynamic registries and routing engine before the HTTP server or delivery workers start.

## Consequences

- A fresh installation is immediately usable with the defaults in `config.yaml`.
- Migration 009 can clear incompatible unshipped destination, profile, and route rows; configured defaults return on the next startup.
- Operator-managed database state survives restarts even when file defaults differ.
- Invalid persisted configuration fails startup before webhook ingestion or delivery begins.
- Defaults are table-scoped: an empty table is seeded even when another dynamic table already contains rows.
