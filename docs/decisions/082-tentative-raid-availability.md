# ADR 082: "Unsicher" as a second raid availability

Status: Accepted. Amends ADR 033 (absences) and ADR 081.

## Decision

Besides "kann nicht kommen", players can say they are **not sure yet** whether they can come. The
owner asked that raid leads can set it for players too.

- `raid_absences.status` is `DECLINED` (existing rows and the default) or `TENTATIVE`
  (migration 071). One row per player and raid, so a player is either declined, unsure, or neither.
  `PUT /api/v1/raids/{id}/absences/{playerID}` takes an optional `status`; missing means `DECLINED`,
  so existing clients keep working. Unknown statuses are `400 invalid`.
- Permissions and deadlines are unchanged: players change their own status until the raid starts;
  `RAIDASSISTENT` and up can set or clear either status for anyone while the raid is active.
- **EP is unaffected by "unsicher".** Only `DECLINED` players are offered for "keine EP" when
  archiving (ADR 021); a player who was unsure and played is treated like everyone else. The bench
  shortcut on the roster ("Auf die Bank setzen") likewise stays for declined players only.
- Audit: setting "unsicher" is logged as `absence_tentative` ("Als unsicher gemeldet"), switching back
  to declined as `absence_declare`, clearing either as `absence_revoke`.
- UI: "Unsicher" (amber badge) on the overview cards, the raid signup tab ("Ich bin unsicher",
  "Sicher dabei"), the staff list "Absagen & Unsicher" with "Als unsicher markieren", the player list
  in signups and the roster board. A free-text note works like the decline reason.

## Consequences

Raid leads see who is still undecided before building the roster. Existing absences are unchanged.
