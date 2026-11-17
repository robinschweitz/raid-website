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
   or the name, or the one that does belongs to the claimant or to a player without a login.
   The profile is reloaded first (FR-7.4). A character of a player without a login is moved like
   a code claim (raid snapshots stay, FR-4.7). The write is audited as `claim_character`.
3. **Otherwise a request.** A character that belongs to another player with a login, and every
   claim of a GUEST, still creates a request that an Admin answers with a code. An open request
   for the profile blocks the claim (`409 registration_request_exists`, FR-4.2).
4. **Requester shown.** The Admin request list names the player who made a claim request.

## Consequences

- `POST /api/v1/characters/claim` answers `{"data": {"claimed": true, "character": …}}` (200) or
  `{"data": {"claimed": false, "request": …}}` (201). The older
  `POST /api/v1/characters/claim-requests` keeps creating a request only.
- Taking a character from a player without a login needs no Admin. Such players are imported or
  quick-added; their history (loot, EP, signups) stays with them, as with a code claim.
