# ADR 065: Normalize existing legacy loot Import IDs

Status: Accepted by owner, 2026-10-15.

## Decision

- Existing historical `legacy-*` Import IDs may be normalized once to
  `legacy-award-<website-loot-award-id>`.
- The old mapping remains immutable. A new mapping and an append-only
  supersession record make the normalized ID active.
- The operation is dry-run-first, requires an ADMIN actor, reason, matching
  non-ID content hash, and a paused automatic scheduler.

## Consequences

This amends ADR 064's rule that existing IDs are never replaced. Only an
explicitly selected legacy prefix is eligible; any missing, duplicate, changed,
or mismatched source row blocks the complete operation.
