# ADR 0009: Watcher Release Workflow

## Decision

Webhook Hub publishes GitHub releases with a single Windows amd64 NSSM-ready binary artifact and a Watcher `version.json` manifest.

The release workflow uses semantic version tags (`vX.Y.Z`), packages `webhook-hub.exe`, and publishes `version.json` with the service key `webhook-hub`.

## Reason

Watcher prefers manifest-first release discovery. Publishing `version.json` gives operators a stable setup and polling contract instead of relying on release asset name inference.

Webhook Hub is a single-binary service, so one deploy target is the correct version-1 release shape. The Watcher health hint uses `/health/ready` because readiness reflects database/runtime availability better than liveness.

## Consequences

- Release commits on `main` or `master` create semver tags only for releasable conventional commit types, or for manual `workflow_dispatch` runs.
- Manual workflow runs default to a patch release unless `force_bump` is set.
- The binary has a `main.Version` variable stamped through linker flags and logged on startup.
- Watcher can consume either the repository URL or the direct release manifest URL.
