# ADR 0011: Single-Event Renderer Profiles

## Decision

Renderer profiles are named, reusable templates for exactly one normalized source/event contract.

Each profile has:

- a unique profile name,
- one `source`,
- one event `key`,
- destination-specific templates for that event.

Profiles are not groups of bindings across many sources or many events.

## Reason

Each normalized event owns a distinct typed payload. Combining many event contracts inside one profile makes template validation ambiguous and encourages untyped metadata access.

The runtime must validate renderer templates against the selected event definition, including the payload fields declared by that event. Templates should use typed payload paths such as `.Payload.attempt.target_version` or `.Payload.workflow_run.name`, not ad hoc metadata lookups.

## Consequences

- A destination may reference several profiles by name, with at most one profile for each source/event contract.
- The profile is used only when the delivery event matches the profile `source` and `key`.
- If the event does not match the profile, delivery falls back to the built-in renderer.
- Unknown profile names, duplicate source/event selections, and profiles without a template for the destination type are invalid.
- Profile validation rejects unknown context fields, undeclared payload fields, whole-context or object access, functions, variables, ranges, associated templates, and `.Metadata` access.
- Adding new event payload fields requires updating the event definition catalog first.
