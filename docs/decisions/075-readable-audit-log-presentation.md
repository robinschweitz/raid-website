# ADR 075: Readable audit-log presentation

Status: Accepted. Amends ADR 074.

## Decision

The Admin audit-log page presents audit entries as domain events instead of exposing their stored
JSON as the primary view. It translates known entity types, actions, values, and changed fields
into German labels and uses names from the audit snapshots for characters, loot, players, and
raids where available.

Technical IDs, timestamps, and unchanged default values are omitted from the primary field
comparison. The original before/after JSON remains available in a separate, collapsed debugging
section for each entry.

## Consequences

Admins can understand ordinary history without parsing database-shaped JSON while retaining the
complete immutable snapshots for diagnosing unusual cases. This is a presentation-only change;
the audit schema, API data, and append-only semantics do not change.
