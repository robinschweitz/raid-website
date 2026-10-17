# ADR 076: Remove participants from planned and open raids

Status: Accepted

## Context

A raid lead could add a character to the roster (`Raidaufstellung`) but had no way to take one out
again: the roster form only corrects role, spec and flags, or moves a row to the Ersatzbank. A wrongly
added or no-longer-coming character stayed in the roster until the raid was archived.

## Decision

The owner requested a removal action and decided:

1. **Only planned and open raids.** `DELETE /api/v1/raids/{id}/attendance/{recordID}` works while the
   raid is `PLANNED` or `OPEN` and not archived. Completed, cancelled and archived raids keep their
   roster; their corrections still go through the audited correction path (ADR 018, ADR 032).
2. **Hard delete.** The roster row is deleted, not hidden, so the roster is not cluttered with
   removed people. The audit log keeps the removed row (`entity_type = attendance`,
   `action = remove`, full row as `before`). Quick-add idempotency records that point at the row are
   deleted with it. This is a deliberate, owner-requested exception to "archive by default" (ADR 007)
   for unfinished raids only: no EP has been granted for them yet (ADR 021), so no ledger history is
   affected.
3. **Loot blocks removal.** If the character (or, for awards without a character, the player) still
   holds unwithdrawn loot in this raid, removal is rejected with `409 attendance_has_loot`. The loot
   must be withdrawn first.
4. **Permission.** `RAIDASSISTENT` and higher, the same role that manages the roster (ADR 010).
5. **Signups.** Acceptance is derived from the roster (ADR 033), so a removed character's signup
   automatically shows as pending again and can be accepted again.

The UI offers `Aus dem Raid entfernen` in the roster edit form of planned and open raids, after a
confirmation prompt.
