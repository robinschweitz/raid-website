# ADR 061: Sync mapping adoption

Status: Accepted by owner, 2026-10-14.

## Decision

- An ADMIN first calls `POST /api/v1/sync/adopt/dry-run`, which writes nothing and returns exact raid matches plus all existing player and character candidates for every unresolved sheet identity.
- `POST /api/v1/sync/adopt` requires a nonblank reason. It adopts exact singleton matches and administrator-selected player/character candidates atomically; ambiguous identities require a valid explicit selection.
- Every newly adopted raid, player, or character mapping writes an audit-log decision with the actor and reason.
- Synchronization and website-to-sheet delivery use the dedicated sync mappings, never immutable `legacy_raid_mappings`. A raid may therefore retain historical import provenance from a different sheet.
- Loot sync state retains its append-only history but no longer has a foreign key to immutable import mappings; new state records retain the adopted raid ID.

## Consequences

The admin can inspect candidates without writes, then resolve ambiguous ownership explicitly. Existing import provenance and its foreign-key history remain unchanged.
