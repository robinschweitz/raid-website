# ADR 045: RaidSync addon exchange for raid invites and recorded loot

Status: Accepted

## Decision

The website exchanges raid data with the in-game addon RaidSync (WoW 3.3.5a, maintained in the
separate `wow-addons` repository). WoW addons have no network access, so the exchange is plain text
that staff copy and paste, like the registration-code export used by InviteMailer.

The format is line-based, `|`-separated and versioned by its first field:

- Invite export (website → addon): header `RS1|INV|<raidID>|<raidName>|<raidSize>`, then one line per
  roster entry `<characterName>|<TANK|HEAL|DD>|<class>|<bench 0/1>`.
- Loot export (addon → website): header `RS1|LOOT|<raidID>|<recorder>|<startUnix>`, then one line per
  recorded drop `<unixTime>|<npcID or ->|<itemID>|<recipient or ->`. `-` as recipient means the drop
  was not handed out.

Invites:

- The invite export lists the saved raid roster (attendance), not signups. Bench entries are only
  included on request. Names are the current character names, because roster snapshots keep the name
  from accept time and a renamed character cannot be invited under its old name.
- Like every `GET` endpoint it is readable by any authenticated user; it contains nothing the roster
  endpoint does not already show.

Loot:

- Loot is distributed in game by master loot. The addon records only boss hint, item and recipient;
  the award method is chosen by raid staff in the website's import preview. The preview defaults to
  `MAIN`, and to `SECOND` for armor tokens, consistent with ADR 028.
- Import is a preview followed by an explicit apply, both RAIDASSISTENT and up. The preview never
  writes. Loot can only be imported into planned or open raids that are not archived ("after the raid,
  before archiving"); archived history keeps using ordinary audited corrections.
- The boss comes from the catalog: an item dropped by exactly one boss of the raid's instance and size
  determines it; otherwise the creature the addon saw die picks the matching candidate; otherwise staff
  choose. Multi-creature encounters map their creatures to the catalog encounter keys.
- Recipients must already be in the raid roster, matched case-insensitively by current or snapshot
  character name. Missing raiders are added through quick-add first; the import never creates roster
  entries.
- Each applied line is written through the ordinary loot path, so catalog validation, roster checks,
  item locks (which may move the award to the lock owner) and Enhanced costs apply exactly as for manual
  entry. Each line uses a stable request ID derived from the raid, the recorder and the line, so
  importing the same export again, by any staff member, creates nothing new. Every line is reported as
  created, already imported or failed; nothing is overwritten.

## Consequences

Raid leaders invite the roster and record loot in game without retyping names or items. Format changes
need a new version prefix and matching changes in both repositories. Raid difficulty (normal/heroic) is
still not stored on raids, so the addon does not set it.
