# ADR 097: A roster place counts as a signup, and staff see the signup list first

Status: Accepted. Amends ADR 081.

## Context

The overview panel "Meine nächsten Raids" (ADR 081) derived a player's status only from the
player's own signups. When the raid lead put one of the player's characters into the roster
directly, the player still saw "Noch nicht angemeldet". On the raid page the "Anmeldungen" tab
showed the member form first and the list that staff work with at the very end, without the
class of each character. The owner asked on 2026-10-24 for both to change.

## Decision

- The status uses the roster as well: a roster row of the player shows "Bestätigt" with that
  character, with or without a signup. Absences still win, as before.
- The "Anmeldungen" tab lists one compact row per player who signed up, stands in the roster, or
  has an absence, grouped pending, in the roster, tentative, declined, with counts per group.
  Every character shows class emblem, class, and spec. Roster rows without a signup appear as
  "Von der Raidleitung eingetragen".
- For staff the list comes first; their own signup and absence form is folded under "Eigene
  Anmeldung". Members keep their form at the top.

Rules: FR-11.3, FR-12.14.
