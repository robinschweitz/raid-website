# ADR 090: Correcting a character's player owner

Status: Accepted.

## Context

An Admin can accidentally attach a character to the wrong player while maintaining the roster.
There was no correction path: the character editor intentionally excludes `player_id`, and account
merging is destructive and transfers every account reference.

## Decision

- **Dedicated Admin correction.** `POST /api/v1/characters/{id}/transfer` moves one live
  character to an existing player. It is separate from the regular character PATCH and is audited
  on the character with before/after owner names.
- **Current ownership only.** The character's wishlist rows update their denormalized `player_id`
  in the same transaction. Raid attendance and loot remain their recorded snapshots; Enhance
  Points, item locks, and signup records are not transferred.
- **User interface.** An Admin opens the incorrectly assigned player's `Charaktere` tab, uses
  `Verschieben` on the character, selects the correct player, and confirms the action.
