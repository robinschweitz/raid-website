# ADR 057 — Continuous Google Sheets reconciliation and website snapshots

Status: Proposed (requires review before enabling in production)

## Context

The existing legacy importer requires an administrator to preview and merge each new
source version. It only bootstraps the first `ICC25 Raidsetup` into a planned raid
once. The owner now wants ongoing synchronization, including the current roster.

Historical Enhance Points are append-only; archived raids have audited corrections
and no replay of finalization. The source sheet does not contain stable IDs or
timestamps for every editable website entity. Blind last-writer-wins bidirectional
updates could overwrite corrections or recreate historical EP and loot awards.

## Strict copy-only boundary (owner clarification)

The original spreadsheet `1N5EgBixB1RNxZagK5d0ie_DMTcYmznpvW5UzuyEWgRU`
is **never** a write or synchronization target. The owner explicitly permits
changes **only** to the working copy
`10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8`.
The copy has all 17 tabs and is the sole permitted source/export destination.
An explicit ID gate rejects the original or any unrelated Sheet.

Prior to this clarification a single `Loot Archive!G1` header was mistakenly
written to the original. It is not silently reverted without owner approval.
The same header is present in the working copy; all subsequent writes must
exclusively target the copy.

## Implemented direction and conflict policy

- The server polls periodically after opt-in. The existing historical importer
  handles `Raid Archive`, `Loot Archive`, `Charaktere`, `Punkte` via its
  preview-hash/transaction/identity rules. New source rows can be merged;
  conflicting edits are stopped and exposed, not applied.
- `ICC25 Raidsetup` A:F is read at each successful import cycle. After an
  administrator's initial setup import, it is matched to the **single** active,
  unarchived PLANNED or OPEN raid in that imported series. New, unambiguously
  known player/character combinations are added with an audit entry.
  Changes to the same participant, deletions, unknown characters and multiple
  active raids are not auto-applied. Incompatible rows stop the entire setup
  merge. No archival EP rewards are recalculated.
- The application exports IDs and current database views in distinct
  `Website …` tabs: raids, raid setup, attendance, loot, characters, EP
  transactions, and series. Its export cannot edit the source four archive
  tabs or the original setup. The export protects against manual changes to
  previously published mirror tabs using persisted fingerprints.
- Both reads and writes use a Google service account and the Sheets API.
  Credentials exist only on the server; API configuration and sync commands
  are Admin-only. The entire feature is disabled unless explicitly configured.
- Errors and last successful sync are persisted and visible in the
  administration UI. Writes to SQLite and Google are not a distributed
  transaction; retries are idempotent and reported.

## Boundaries / further work

This is a **non-destructive bidirectional data exchange**, not an editable
two-way master for every field: archive changes and edits to Website snapshot
tabs cannot yet be written back to the other system. Making both sides editable
for history requires stable external IDs, field ownership, conflict resolution,
and an explicitly approved deletion/correction workflow. Do not enable an
implicit last-writer-wins strategy. This ADR amends the one-off import language
of ADR 008 and ADR 029 only for the additive operations described here.
