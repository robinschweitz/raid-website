# ADR 029 — Archived import additions and explicit current raid setup

Status: Accepted by owner, 2026-10-05.

## Decision

- Historical sheet raids are imported as COMPLETED with `archived_at`. A later
  reviewed merge may classify older, provenance-mapped COMPLETED imports as archived,
  with actor/reason audit snapshots and a report. This does not finalize rewards again.
- ADMIN legacy imports may append missing attendance and loot to archived COMPLETED
  raids with matching source/sheet provenance. A nonblank reason is mandatory.
  Existing records are never replaced. Cancelled raids remain immutable. Ordinary
  attendance/loot edit endpoints still reject archived records.
- Imported additions use the existing audited correction transaction: only EP
  eligibility differences are appended, tokens are derived, and historical enhanced
  awards never recreate EP costs. Opening balances and ledger history are retained.
  This narrowly supersedes the archived-import restriction in ADR 018–021.
- Fetch `ICC25 Raidsetup`, columns A–F: position, character, player, class, spec,
  role. Import the first numbered roster block only. Empty slots and unavailable specs
  are reported; specs are not invented. Later notes/secondary tables are reported and
  excluded. Unknown roles, invalid classes, conflicting ownership, and duplicate
  positions/characters block the merge. An incomplete spec must be filled before
  the ordinary archival action can succeed.
- Current setup creates a separate PLANNED ICC25 raid and an initial raid series,
  atomically with the historical import. The series and first occurrence use explicit
  administrator configuration, never inferred dates or defaults in the domain.
  The owner supplied Oxora, first date 2026-10-05, weekly Monday at 19:00. The import
  form exposes these values and Europe/Berlin (the sheet timezone) for review/editing.
  The first date is preserved even if the import happens after its start time.
- Setup creation is opt-in in the request (enabled in the form) and can occur after
  an earlier historical import. A permanent source/sheet setup mapping prevents repeat
  bootstrap across changed snapshots. Existing mapped configuration and roster are
  preserved; unproven name collisions are reported instead of overwritten or adopted.
- Preview hashes include setup CSV and the supplied schedule configuration. Preview
  and merge report the planned series, date, leader, roster and counts. All repositories
  share the import transaction so failures roll back both setup and history.

## Scope

This decision does not implement boss-kill tracking or change live raid completion.
Those require actual encounter records; placeholder dashboard progression is not a
source of truth for completion.
