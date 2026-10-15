# ADR 064: Historical loot Import ID baseline

Status: Superseded by ADR 066, 2026-10-15.

## Decision

- Historical `Loot Archive` rows without an `Import ID` receive the permanent ID
  `legacy-award-<website-loot-award-id>` once, through a dry-run-first admin
  operation.
- The operation writes only empty `Import ID` cells. Existing IDs are never
  replaced.
- Existing no-ID mappings remain append-only. A new ID mapping is appended and
  an append-only baseline record marks it as the active successor.
- Applying requires the non-ID Loot Archive content hash from the dry-run, an
  active ADMIN actor, an audited reason, and an explicit confirmation that
  automatic Google Sheets synchronization is paused.
- Sheet writes are verified before the database transaction. If a failure occurs
  after Sheet writes but before the transaction, rerunning safely appends the
  missing mappings only when every generated ID and historical source row still
  match exactly.

## Consequences

The baseline changes neither loot awards nor EP balances. Subsequent incremental
sync uses the new permanent IDs for corrections, withdrawals, and restorations.
Rows that cannot be matched one-to-one are conflicts and block the entire run.
