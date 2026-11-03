# ADR 111: The player list is a staff page

Status: Accepted.

## Context

Since the public "EP-Stand" page (ADR 110) lists every active player, the "Spieler" list shows
members nothing more than names and an active state. Visitors saw the menu entry but landed on
the login page.

## Decision (owner, 2026-11-03)

- The page `/players` moves from the main navigation into the "Administration" section, which
  RAIDASSISTENT and higher see. Other roles opening `/players` get a short German note instead
  of the list.
- "EP-Stand" stays in the main navigation for everyone.
- Player profiles stay open to every signed-in user, as before: members reach them through the
  names on "EP-Stand" and their own name in the top bar. The loot and EP history therefore stays
  checkable by every member (FR-30.4).
- The API does not change: the public player list (FR-1.2) still feeds names across the site.
  The page restriction is navigation, not a permission (SC-001 is unaffected).
