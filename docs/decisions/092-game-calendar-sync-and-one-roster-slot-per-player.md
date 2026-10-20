# ADR 092: Game calendar sync and one roster slot per player

Status: Accepted. Amends ADR 033 (signups), ADR 045 and ADR 085 (invite export), ADR 037 (quick-add).

## Context

RaidLedger creates the raid's event in the WoW game calendar from the website's invite list, but the
list only held the roster, the event carried no statuses, and answers given in the game calendar never
reached the website. The owner asked that the calendar invite list and its statuses can be synced
easily in both directions: website answers into the game, calendar answers onto the website.

The owner also asked that a player can no longer end up in a raid's roster with two different
characters by accident. Before this change only signup acceptance (ADR 033) enforced one character per
player; manual roster additions, the addon roster import and quick-add for an existing owner did not.

Owner decisions (2026-10-20):

- Everyone who answered on the website goes into the calendar event, each with a status.
- A calendar acceptance becomes a **pending signup** (the raid lead accepts as usual), not a roster slot.
- Calendar answers reach the website through **preview, then apply**, like the roster and loot imports.
- The RaidLedger change ships through the bundle in this repository; the owner carries it over to
  `wow-addons` (and `addon-forge`).

## Decision

### One roster slot per player

- `raidcorrection` rejects adding a roster row (bench included) for a player who already has a row
  with another character in that raid, and moving an existing row onto such a player, with
  `ErrPlayerInRoster` (`PlayerInRosterError` names the occupying character). HTTP answers `409
  player_already_in_roster` with that character's name. Every ordinary path goes through this check:
  manual attendance, signup acceptance, the addon roster import (reported per character) and quick-add.
- Switching the character inside the existing slot (row edit, signup "Auf diesen Charakter wechseln")
  stays possible.
- The rule is enforced in the service, not with a unique index: rows that predate it (legacy imports,
  account merges, admin character transfers under ADR 091) remain and stay correctable; the check only
  runs when a row is added or changes player. The roster import preview flags known characters whose
  player already holds a slot and does not preselect them.

### Website → game: answers in the invite list

- `RS2;INV` keeps its roster lines unchanged (four fields). After them follow players **without a roster
  slot** who answered on the website, with a fifth field: `SIGNUP` (pending, not withdrawn signup),
  `TENTATIVE` or `DECLINED` (absence, ADR 082). An absence wins over a pending signup. The character is
  the pending signup's (main character first), otherwise the player's active main (else oldest)
  character; the role follows the signup's spec, otherwise the main spec. Order: signups, unsure,
  declined, each by name. Players without an active character are left out.
- RaidLedger before 2.6.0 reports five-field lines as unreadable and skips them, so old versions keep
  inviting the roster.
- RaidLedger 2.6.0 never invites answer lines into the raid group. It puts everyone on the list into
  the calendar event and sets statuses: roster `CONFIRMED`, bench `STANDBY`, `SIGNUP` → `ACCEPTED`,
  `DECLINED` → `DECLINED`. The 3.3.5 calendar has no "tentative", so unsure players stay `INVITED`.
  **SYNC** (`/ledger calendar sync`) does the same for the existing event: it invites who is missing and
  sets statuses; it runs automatically once a newly created event is confirmed.
- A player's own calendar answer is never overwritten: website answers only fill in "no answer yet"
  (`INVITED`, `NOT_SIGNEDUP`), and a roster slot never replaces a refusal (`DECLINED`, `OUT`); such
  refusals are named in chat instead. Editing needs the event's creator or a moderator.

### Game → website: calendar answers

- RaidLedger's **ANSWERS FOR THE WEBSITE** (`/ledger calendar export`) writes
  `RS2;CAL;<raidID>;<recorder>;<exportUnix>` and one line `<name>;<answer>` per invite, the answer being
  one of `invited accepted declined confirmed out standby signedup notsignedup tentative`.
- `POST /api/v1/raids/{id}/addon-calendar-import/preview` and `.../addon-calendar-import` are
  RAIDASSISTENT and up, for planned or open, unarchived raids. Names are matched case-insensitively
  against all characters; unknown characters are shown, never created.
- Proposed actions: `accepted`/`signedup` → a pending signup with the character's main spec, unless the
  player already has a roster slot or that character a pending signup; an existing website absence is
  removed on apply (the preview says so). `declined`/`out` → absence `DECLINED` unless already
  declined; the roster is not changed (absences never change it). `tentative` → absence `TENTATIVE`.
  `confirmed`/`standby` (set by the raid lead) and missing answers change nothing. Only one action per
  player is proposed.
- Apply recomputes the preview and executes only selections that match a currently proposed action;
  anything else is reported as `not_proposed`. Each change goes through `raidsignup`, which audits it.
- Staff signing up on behalf of a player is the new `raidsignup.SubmitForPlayer` (audit action
  `submit_for_player`): RAIDASSISTENT and up, active raid, valid spec for the class; it does not need an
  open signup window (staff may also accept after closing, ADR 033). An existing pending choice is left
  unchanged; a withdrawn one is submitted again.

## Consequences

- Raid leads see website answers in the game calendar and bring calendar answers back with one
  copy-and-paste, without overwriting anyone's own answer on either side.
- RaidLedger 2.6.0 needs a website deploy (ADR 085). The calendar sync was tested offline against a
  simulated 3.3.5 calendar; whether the client lets an addon set invite statuses from a timer
  (`CalendarEventSetStatus`) still needs a live run on the test realm. The calls are wrapped in `pcall`,
  so a refusal cannot break event creation.
- There are no new screenshots of the calendar column yet; the addon page describes it in prose.
