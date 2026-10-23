# ADR 088: "Aktueller Raid" link and an Admin-set boss order

Status: Accepted.

## Context

After the raid of 2026-10-19 the owner asked for a direct way to the newest raid, and found the
boss lists in the wrong order: the boss picker for recording loot listed an instance's bosses by
their internal catalog ID, and a raid's loot list showed its boss groups in the order the loot was
recorded. The owner chose a navigation entry for the newest raid, and an order an Admin can set,
used both by the boss picker and by the loot list.

## Decision

- **Aktueller Raid.** The main navigation (desktop "Aktueller Raid", mobile "Aktuell") opens
  `/raids/current`, which forwards to the raid that `GET /api/v1/raids/current` names (any signed-in
  user). The server picks the **newest raid that has begun**, meaning its real start, else its scheduled
  start, is not in the future, so tonight's raid counts from its start time and stays current until
  the next one begins. Imported history without either start counts from its completion. If
  none has begun, it picks the **next raid to come**. Cancelled raids and raids without any time
  are skipped; with no raid at all the answer is 404 `no_current_raid`.
  The forward replaces the history entry, so "back" does not return to the link.
- **Boss order.** `loot_catalog_boss_order` (migration 074) stores a position per boss. It is guild
  configuration beside the bundled catalog (ADR 027), never part of it. `PUT
  /api/v1/loot-catalog/boss-order` (Admin) takes an instance and **every boss of it exactly once**
  (else 422 `boss_order_invalid`), replaces that instance's positions and writes an audit entry
  (`boss_order`, before and after order). The Admin edits it with ↑ / ↓ on the Beute page.
- Every boss list the API returns (`/loot-catalog`, `/loot-catalog/bosses`) follows the order;
  bosses without a position follow in catalog order. A raid's loot (`GET /raids/{id}/loot`)
  is sorted by its catalog boss in that order (loot without a catalog boss last), so the raid
  page's boss groups follow it without an extra request.
