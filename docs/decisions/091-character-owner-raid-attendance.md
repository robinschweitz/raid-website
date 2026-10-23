# ADR 091: Character owner corrections update raid attendance

Status: Accepted. Amends ADR 090.

## Context

ADR 090 intentionally left raid attendance unchanged when an Admin corrected a character's owner.
That left roster rows displaying the old player despite the character now belonging to the new one.
The owner requires the raid roster to show the correct player after this correction.

## Decision

- **Roster ownership follows the character.** Character transfer updates `player_id` on every
  `raid_attendance` row for that character, including archived raids. Each roster-row correction is
  audited with its own before/after snapshot in the same transaction as the character transfer.
- **Conflicts stop the correction.** If the target player already has the character in the same
  raid roster, the transfer is rejected and nothing changes.
- **Other history stays put.** Loot awards, Enhance-Point transactions, item locks, and signup
  records are not transferred. Wishlist ownership still follows the character under ADR 090.
