# ADR 028: Legacy Loot Import Preserves Sheet Priority, Token Semantics, and Catalog Boss

## Status
Accepted.

## Context
The Loot Archive has one `Priority` column, and its values were not carried into the domain. Only
`Main` produced main-spec loot; `Second`, `Token`, `Enhanced`, and `BoE` all collapsed into an
off-spec award because the plan only tracked a boolean. `Enhanced` and `BoE` awards therefore lost
their meaning, and the sheet's boss was never recorded at all, even though the bundled catalog
(ADR 027) can name the source of most items.

## Decision
The sheet's `Priority` is the authority for how a historical award was given and maps onto the
recorded award method: `Main` → `MAIN`, `Second` → `SECOND`, `Enhanced` → `ENHANCED`,
`BoE` → `BOE`, `Token` → `SECOND`. An unknown or empty priority is recorded as `SECOND` and
reported as a warning, never silently treated as main-spec loot.

An armor token never blocks the +10 EP archive reward (ADR 004), whatever priority the sheet
recorded, so a token award is stored with a non-main method and a warning. The token ledger stays
derived from the awarded item (ADR 014); a `Token` priority on an item that is not one of the three
badges derives no token and is reported.

An imported `ENHANCED` award keeps its method and counts as main-spec loot, but never charges EP:
the historical cost is zero and no enhanced-cost transaction is appended.

The boss of an imported award is derived from the bundled catalog when the item belongs to exactly
one boss. An item that drops from several bosses, or is not in the catalog, proves nothing: the boss
stays unknown and the row is reported. Item names and other free text are never rewritten.

The sheet raid key carries the catalog context (`ICC25-<size>-<sequence>`); only that proven shape
is decoded, and only for raids this import creates. Any other key leaves the raid unassociated.

## Consequences
- Priorities survive the import, so an `Enhanced` or `BoE` row is no longer displayed and counted
  as an ordinary second-spec award.
- Historical Enhanced rows can never charge a player for a roll that happened outside the website.
- Boss attribution is only recorded where the catalog proves it; unresolved rows are visible as
  warnings in the dry-run report instead of being guessed.
- Importing the same source twice stays idempotent, so already-imported rows keep their original
  method and boss until an explicit audited correction changes them.