# ADR 045: RaidSync addon exchange for raid invites, roster and recorded loot

Status: Accepted

## Decision

The website exchanges raid data with the in-game addon RaidSync (WoW 3.3.5a, maintained in the
separate `wow-addons` repository). WoW addons have no network access, so the exchange is plain text
that staff copy and paste, like the registration-code export used by InviteMailer.

The format is line-based, `|`-separated and versioned by its first field:

- Invite export (website → addon): header `RS1|INV|<raidID>|<raidName>|<raidSize>`, then one line per
  roster entry `<characterName>|<TANK|HEAL|DD>|<class>|<bench 0/1>`.
- Roster export (addon → website): header `RS1|ROSTER|<raidID>|<recorder>|<exportUnix>`, then one line
  per character the addon saw in the raid `<name>|<classToken or ->|<firstSeenUnix>`. Characters who
  left again stay in the list.
- Loot export (addon → website): header `RS1|LOOT|<raidID>|<recorder>|<startUnix>`, then one line per
  recorded drop `<entryID>|<unixTime>|<npcID or ->|<itemID>|<recipient or ->|<state or ->`. The entry ID
  numbers the drops of one recording and never changes. `-` as recipient (with state `-`) means the
  drop was not handed out. The state says how the recipient is known: `given` (master-loot
  assignment, loot message or completed trade), `held` (the recorder looted it to himself and has not
  traded it on yet) or `manual` (set by hand in the addon).

Invites:

- The invite export lists the saved raid roster (attendance), not signups. Bench entries are only
  included on request. Names are the current character names, because roster snapshots keep the name
  from accept time and a renamed character cannot be invited under its old name.
- Like every `GET` endpoint it is readable by any authenticated user; it contains nothing the roster
  endpoint does not already show.

Roster:

- Raids are often filled up in game, e.g. via the world chat. The roster import is a preview followed
  by an explicit apply, both RAIDASSISTENT and up, for planned or open raids that are not archived.
- The preview matches each name case-insensitively against the roster (current or snapshot name), then
  against all known characters, also case-insensitively (an exact-case match wins, otherwise the oldest
  character). Known characters get a suggested role (their latest attendance,
  otherwise DD) and a late-join suggestion when first seen more than 15 minutes after the raid start.
  Roster members (not bench) the addon never saw are listed for information only.
- Apply adds only selected known characters that the export names, through the ordinary attendance
  path. Characters already in the roster are reported, not duplicated. Unknown characters are not
  created automatically: the preview hands their name to the Rising-Gods quick-add, so staff confirm the
  profile and the owning player.

Loot:

- Loot is distributed in game by master loot, often to the loot master himself, who trades the items
  to the roll winners later. The addon follows those trades: a completed trade hands each traded item
  the recorder still holds to the trade partner. Trades between two other players are invisible to the
  addon; the recorder corrects such drops by hand, or staff pick the recipient in the import preview.
- The addon records boss hint, item, recipient and state; the award method is chosen by raid staff in
  the website's import preview. The preview defaults to `MAIN`, and to `SECOND` for armor tokens,
  consistent with ADR 028.
- Import is a preview followed by an explicit apply, both RAIDASSISTENT and up. The preview never
  writes. Loot can only be imported into planned or open raids that are not archived ("after the raid,
  before archiving"); archived history keeps using ordinary audited corrections.
- The boss comes from the catalog: an item dropped by exactly one boss of the raid's instance and size
  determines it; otherwise the creature the addon saw die picks the matching candidate; otherwise staff
  choose. Multi-creature encounters map their creatures to the catalog encounter keys.
- The recorded recipient is a suggestion, matched case-insensitively by current or snapshot character
  name. Staff may pick any roster character instead; recipients outside the roster are rejected, so
  missing raiders are added through the roster import or quick-add first. Held drops are flagged and not
  preselected.
- Each applied line is written through the ordinary loot path, so catalog validation, roster checks,
  item locks (which may move the award to the lock owner) and Enhanced costs apply exactly as for manual
  entry. Each line uses a stable request ID derived from the raid, the recorder, the recording start and
  the entry ID, not from the recipient. Unlike ordinary loot request IDs, which only make one staff
  member's retries idempotent, these `raidsync-` IDs are scoped to the raid: the loot write transaction
  looks them up by raid while it holds the write reservation, and the unique index
  `idx_loot_raid_request` on `(raid_id, request_id)` backs this up. Importing the same drops again, by
  any staff member, concurrently, and even after a later trade changed the recipient, creates nothing
  new; the preview warns when the export now
  names a different recipient, which staff change through the ordinary loot correction. Every line is
  reported as created, already imported or failed; nothing is overwritten.

## Consequences

Raid leaders invite the roster, report late additions and record loot in game without retyping names or
items. Format changes need a new version prefix and matching changes in both repositories. Raid
difficulty (normal/heroic) is still not stored on raids, so the addon does not set it.
