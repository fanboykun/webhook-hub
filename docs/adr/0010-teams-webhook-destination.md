# ADR 0010: Microsoft Teams Webhook Destination

## Decision

Webhook Hub supports Microsoft Teams as a delivery destination using Teams incoming webhook URLs.

The destination type is `teams`. It uses the existing encrypted `webhook_url` / `webhook_url_env` destination secret field and sends MessageCard-compatible JSON payloads.

## Reason

Teams incoming webhooks have the same operator-owned URL shape as Slack incoming webhooks, so reusing the existing webhook URL field keeps the persistence and dynamic destination contract small.

Teams still needs its own destination type, sender, renderer fallback, retry classification, and renderer-profile template fields because message formatting and downstream error behavior are provider-specific.

## Consequences

- Operators can define static or API-managed Teams destinations with `type: teams`.
- Routes can target Teams destination IDs the same way they target Slack and Telegram destinations.
- Teams templates use `teams.title` and `teams.body` in renderer profiles.
- HTTP 429 and 5xx Teams responses are retryable; other non-2xx responses dead-letter after the attempt is recorded.
