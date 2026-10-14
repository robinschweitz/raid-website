# ADR 058 — Safe activation and transactional sheet reconciliation

Status: Accepted for the reviewed safety implementation; production cutover remains pending.

## Context

The owner requested safe activation after an earlier one-off import. ICC25-35
is archived with loot in the sheet, but is active without loot on the website.
Regression tests exposed ignored source changes, late opening balances after
zero imports, partial imports before setup conflicts, and destructive clear/put
exports. ADR 057's website-only archival and copy-only boundaries remain binding.

## Decision

- Configuration enables the Admin API but does not start polling. Polling requires
  the additional explicit `SHEET_SYNC_AUTORUN=true` switch. It defaults to false.
- An Admin-only preflight rolls back the same complete domain transaction used by
  a real run. It reports blockers and never publishes mirrors or saves import
  receipts. The UI requires this preflight before a manual run; the server always
  rechecks the current source independently of the client.
- Fetch one source version per reconciliation. Run preview, historical merge and
  current setup merge in one database transaction. Any domain conflict rolls back
  all changes, including characters, players, ledger entries and source baselines.
- Save source snapshots with successful imports. Automatic runs compare existing
  character, points, attendance, loot-ID and setup rows with these baselines.
  Edits/removals are conflicts; points with an absent or zero opening are never
  inferred as new earned points after an earlier source version or ledger history.
- Legacy imports without a persisted baseline require an explicit cutover review.
  Migrations do not infer a baseline, relabel provenance or modify EP/history.
- Record synchronized setup membership per raid. Removed source participants are
  conflicts. A finalized setup predecessor must not populate its successor from
  a stale source block; the website successor remains authoritative.
- Only the designated copy and the seven Website snapshot tabs are writable.
  Validate existing mirrors before committing source imports, and again before
  export. Replace all changed tabs with one Sheets `batchUpdate` of `addSheet`,
  grid expansion and `updateCells`; clear omitted cells inside that same batch.
  A failed batch does not erase the previous snapshot. A lost successful response
  is retried by accepting remote contents equal to the desired snapshot.
- SQLite and Google are not a distributed transaction. An external failure after
  a committed domain reconciliation is reported and retried idempotently; it must
  not replay domain writes. Concurrent edits between remote validation and batch
  application cannot be prevented by a Sheets compare-and-swap API.

## ICC25-35 cutover prerequisite

The implementation deliberately does not guess whether the actual production EP
balances already include ICC25-35. It does not automatically connect a same-name
active raid to historical sheet rows, assign IDs to historical loot, transfer
Original-to-copy provenance, or archive the real raid. A reviewed production DB
backup and an explicit one-time reconciliation are required before enabling its
polling. Preflight keeps this ambiguous state blocked.

## Validation

Regression tests cover source changes/removals, zero/absent opening balances,
partial setup failure rollback, stale successors, read-only preflight, legacy
baseline blockers, role boundaries, original rejection, atomic export failure and
lost-response retries. Eight ICC25-35 variants were rerun against the saved copy
snapshot in isolated databases. No production database or live sheet was modified
while implementing these fixes.

## References

- https://developers.google.com/workspace/sheets/api/guides/batch
- https://developers.google.com/workspace/sheets/api/reference/rest/v4/spreadsheets/request#UpdateCellsRequest
