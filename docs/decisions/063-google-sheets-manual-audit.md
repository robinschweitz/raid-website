# ADR 063: Google Sheets manual audit

Status: Superseded by ADR 066, 2026-10-15.

## Decision

- An owner-installed Google Apps Script records manual edits to `Raid Archive`,
  `Loot Archive`, `ICC25 Raidsetup`, and `Punkte` in the append-only
  `Website Manual Audit` worksheet.
- Cell edits record timestamp, editor when Google exposes it, sheet, A1 cell,
  event type, old display value, and new display value. Structural changes are
  logged and refresh the script-owned hidden snapshots used for old values.
- Application-originated loot delivery remains separately auditable in the
  `Website Sync Audit` worksheet. The Apps Script does not alter domain data
  or replace server-side audit records.

## Consequences

The spreadsheet owner installs and authorizes the script. Google consumer
accounts can omit the editor identity. The script records future changes only;
it cannot reconstruct prior Google Sheets version history.
