# ADR 089: Loot that went to nobody

Status: Accepted. Amends ADR 070 (RS2 import of `disenchanted` / `bank` lines).

## Context

In the raid of 2026-10-19 items were disenchanted, kept for the guild bank, or nobody wanted
them. The website only knew awards to players, and the RS2 import showed RaidLedger's
`disenchanted` and `bank` lines without recording them anywhere. The owner wanted both: the
import keeps them, and staff can enter such items by hand.

## Decision

- **Separate record, never an award.** `unclaimed_loot` (migration 075) holds raid, optional
  catalog boss (must belong to the raid's instance), item ID and name (the catalog fills the
  name), outcome `DISENCHANTED`, `BANK` or `NOBODY`, the holder's name, notes and the source
  (`manual` or `addon`). It counts for no player: no EP effect, no tally, no token, no item lock.
- **Who.** Everyone signed in reads a raid's list (`GET /api/v1/raids/{id}/unclaimed-loot`);
  RAIDASSISTENT and up add entries (`POST` same path) and remove them
  (`POST /api/v1/unclaimed-loot/{id}/remove`). Cancelled raids take none; archived raids need a
  reason for both. Removing archives the row (`archived_at`, ADR 007). Both writes are audited
  (`unclaimed_loot`).
- **Import.** RS2 `disenchanted` and `bank` lines are now importable as unclaimed loot: no
  recipient or award method, the boss optional, the holder from the line. The line key makes it
  idempotent (unique per raid); a second import reports `already_imported`. A line already
  imported as an award that the addon later moved to DE/BANK still only warns, as before.
  Parked and deleted lines are unchanged: shown only.
- Items left on the corpse are not in the addon export; staff enter them by hand as `NOBODY`.
