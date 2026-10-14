# ADR 058: Admin legacy raid linking

Status: Accepted by owner, 2026-10-13.

## Decision

- An ADMIN may explicitly link one existing website raid to one `google_sheets`
  source, sheet ID, and raid key through the API.
- A nonblank reason is required. The actor, link metadata, and an audit-log entry
  are written atomically with the immutable mapping.
- A raid and source key can each be linked only once. Existing links are read-only.

## Consequences

`GET` and `POST /api/v1/raids/{id}/legacy-link` are ADMIN-only. The endpoint
does not fetch or apply sheet data; it only provides explicit provenance for the
existing legacy synchronization workflow.

This amends ADR 057 by exposing its append-only raid-link persistence through an
explicit administrative operation.
