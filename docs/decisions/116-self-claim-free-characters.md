# ADR 116 — Claiming free characters without a code

- Status: accepted
- Date: 2026-11-17
- Decided by: owner (2026-11-17)
- Amends: ADR 040 (Rising-Gods-backed registration codes)

## Context

Every character claim needed an Admin-issued code delivered in game, also for a player who
already has a login and claims a second character that nobody owns. That is Admin work without
a security gain: the code proves control of the in-game character, but a free character takes
nothing from anyone. The Admin request list also did not say who made a claim request.

## Decision

1. **First login only.** A registration code is needed only for the first login (FR-4.5).
2. **Free characters at once.** A MEMBER or higher who claims a Rising-Gods character gets it at
   once through `POST /api/v1/characters/claim` when it is free: no character holds the profile
   or the name, or the one that does belongs to the claimant. The profile is reloaded first
   (FR-7.4); claiming an own unlinked character links the profile and keeps specs the owner
   chose (FR-6.16). The write is audited as `claim_character`.
3. **Otherwise a request.** A character that belongs to another player, with or without a login,
   and every claim of a GUEST, still creates a request that an Admin answers with a code. When
   another player holds the character, the answer sets `owned_by_other` and the page tells the
   player to contact an Admin. An open request for the profile blocks the claim
   (`409 registration_request_exists`, FR-4.2).
4. **Requester shown.** The Admin request list names the player who made a claim request.

## Consequences

- `POST /api/v1/characters/claim` answers `{"data": {"claimed": true, "character": …}}` (200) or
  `{"data": {"claimed": false, "owned_by_other": …, "request": …}}` (201). The older
  `POST /api/v1/characters/claim-requests` keeps creating a request only.
- A character of a player without a login (imported, quick-added, or anonymised) is never taken
  without an Admin: such a player may be a guild member who has not registered yet, and the
  in-game code is the only proof that the claimant plays the character.
