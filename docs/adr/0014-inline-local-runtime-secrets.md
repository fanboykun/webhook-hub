# ADR 0014: Inline Local Runtime Secrets

## Decision

Root `config.yaml` is ignored by Git and treated as a secret-bearing runtime file equivalent to `.env`. Local operators may configure secrets inline through `admin_token`, `encryption_key`, integration `secret` or `client_secret`, and destination `webhook_url` or `bot_token` fields.

Each inline field has an optional `*_env` counterpart for deployments. The two forms are mutually exclusive for one secret, and missing referenced environment variables fail startup. Application code consumes only resolved fields and must never log either form.

## Consequences

- Local startup does not require exporting a parallel set of environment variables.
- The tracked example contains nonfunctional replacement values, never live credentials.
- Root config must remain ignored, permission-restricted, and outside agent read/edit scope.
- Losing `config.yaml` can lose credentials, so operators must manage their own protected backup.
