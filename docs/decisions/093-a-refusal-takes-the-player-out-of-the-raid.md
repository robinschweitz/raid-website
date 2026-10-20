# ADR 093: A refusal takes the player out of the raid

Status: Accepted. Amends ADR 069 ("Nachtrag: Absage durch Spieler"), ADR 076 (removal permission) and
ADR 081 ("signup kept"); refines ADR 092 (calendar refusals).

## Context

Until now an absence only marked a player: "Aufstellung, Anmeldungen und Bank bleiben unverändert, die
Raidleitung entscheidet" (ADR 069, repeated in ADR 081). With the game calendar import (ADR 092) a
refusal in the game left a player with an open signup, or a roster slot, next to "kann nicht".

Owner decisions (2026-10-20):

- Whoever says "kann nicht" has effectively withdrawn: the signup is no longer open.
- The roster slot does not stay either.
- If the player already holds loot in this raid, the refusal is rejected.
- A calendar answer keeps the reason the player gave on the website.

## Decision

- `raidsignup.Decline` with `DECLINED` does, in one transaction:
  1. remove every roster row of the player in this raid, bench included, through
     `raidcorrection.RemoveAttendance` (hard delete, audit `attendance`/`remove` with the full row,
     ADR 076);
  2. withdraw every open signup of the player (audit `raid_signup`/`withdraw`, actor = who declined).
     Removing the rows first turns accepted signups into open ones (acceptance is derived from the
     roster, ADR 033), so they are withdrawn too;
  3. save the absence as before.
- This applies to every refusal: the player's own (until the scheduled start, ADR 069), staff marking a
  player (while the raid is active) and the calendar import. A player can thereby remove their own
  roster slot, which ADR 076 reserved for staff.
- Loot blocks it: if the character or player holds unwithdrawn loot in this raid, the refusal fails
  with `raidcorrection.ErrHasLoot` → `409 attendance_has_loot` and nothing changes. The calendar import
  reports it per row with code `attendance_has_loot`.
- "Unsicher" (`TENTATIVE`) changes neither roster nor signups. Switching from unsure to declined does.
- Revoking an absence ("Doch dabei", "Absage zurücknehmen") only removes the absence; roster slot and
  signup are not restored, the player signs up again. A calendar acceptance signs up again by itself
  (ADR 092, `SubmitForPlayer` also removes the absence in the same transaction).
- Calendar answers (`declined`/`out`, `tentative`) keep an existing website absence reason; only
  without one they store "Im Spielkalender beantwortet". The preview says which reason stays and that
  the roster slot is removed or the open signup withdrawn.
- The UI asks before a refusal that removes a roster slot or withdraws a signup (own refusal on the
  raid page and in "Meine nächsten Raids", staff "Als abgesagt markieren").

## Consequences

- Roster and signups always match what players said; raid leads no longer clean up after refusals.
- New "in the roster and declined" combinations no longer arise. The archive option "Keine EP für …"
  (ADR 069) stays for existing data but will rarely have candidates.
- A refusal after the raid started (staff only) with loot needs the loot withdrawn first.
