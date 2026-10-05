# ADR 032: In-place editing and reasoned archive corrections

## Decision

The owner requested in-place editing, reasons only for archived changes, and explicitly
authorized RAIDASSISTENT, RAIDLEADER, and ADMIN to correct archived raid roster and loot.

- Edit the selected roster card, loot row, character row, or raid-series card in place,
  with Save and Cancel. No jump to a detached editor or browser prompt for renaming.
- PLANNED, OPEN, and unarchived COMPLETED roster/loot writes need no reason. Archived
  COMPLETED corrections and missing-entry additions require a trimmed nonblank reason.
  Cancelled raids and inconsistent archived non-COMPLETED raids remain unwritable.
- Live-character edits remain Admin-only, without a required reason. Actor, before/after
  audit snapshots, and character name history remain mandatory and transactional.
- Every archived change uses the existing correction transaction: append only the EP
  eligibility delta and token compensation, preserve or compensate enhanced costs, and
  never revive consumed/expired locks. Do not rerun finalization, modify archival timestamps,
  create another successor, or recalculate imported opening balances.
- Import remains Admin-only, preview-first, provenance checked, and non-overwriting.
  Additions to archived history and classification of older completed imports require
  a reason. Additions to active imported raids do not.

This supersedes the reason and archived read-only rules in ADR 006, 018, 020–022, 029–030;
all other accounting, permission, catalog, and audit requirements continue to apply.
