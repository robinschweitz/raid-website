# ADR 070: RaidLedger loot import (RS2) and wishlist export for RaidWishlist (WL2)

Status: Accepted. Extends ADR 045; applies ADR 023 to the addons.

## Context

The guild's raid leads record loot in game with RaidLedger and raid members use
RaidWishlist (both WoW 3.3.5a, maintained in `robinschweitz/addon-forge`, `examples/`). The
ledger runs the roll (`/rnd 100` main spec, `/rnd 99` second spec, enhanced `1+X..100+X`), tracks
items that are still open, and exports its awards as RS2. The wishlist addon imports a member's
wishlist and rolls for them, or prompts them, when the ledger starts a roll on a wished item.

## Decision

Loot import (addon → website):

- `ParseLootExport` reads RS1 (ADR 045) and RS2. RS2 is `;`-separated, because a WoW EditBox treats
  `|` as an escape character:
  - header `RS2;LOOT;<raidID>;<recorder>;<startUnix>;<exportUnix>`
  - lines `<entryID>;<unixTime>;<npcID or ->;<itemID>;<recipient>;<state>;<roll or ->;<ms|os|->;<low-high or ->;<winner or ->`
- RS2 states are `given`, `held`, `manual` (as in RS1), `parked` (another raider keeps the item until
  it is rolled, e.g. a unique item the loot master cannot carry twice) and `deleted` (removed in the
  addon; the addon keeps the line so the website can see it).
- The preview shows roll, spec, range and winner. The default award method is `ENHANCED` for an
  enhanced range (`high = low + 99`, X = `low - 1` from 1 to 50), otherwise `SECOND` for armor tokens
  (ADR 028) and second-spec rolls, otherwise `MAIN`. Staff may still change it.
- A `held` line with a winner suggests the winner as recipient and is preselected; the warning says
  the trade is still open. A `held` line without a winner stays unselected (ADR 045).
- `parked` and `deleted` lines are shown with a warning and are never imported: the preview cannot
  select them and the apply endpoint reports `not_importable`. A line imported earlier and deleted in
  the addon later is not withdrawn automatically; the warning points to the ordinary loot correction.
  Staff decide.
- The website stays the source of truth for EP: the debit of an `ENHANCED` award is X = min(balance,
  50) as before (ADR 023/024). When the addon's range disagrees with that, the preview warns and the
  account value is debited.

Wishlist export (website → addon):

- Each wish carries `spec` (`MAIN` or `SECOND`, default `MAIN`) and `is_enhanced` (only for `MAIN`),
  migration 069. The owner (MEMBER and up) changes spec, enhanced, priority and notes with
  `PUT /api/v1/characters/{id}/wishlist/items/{itemID}`; nobody else may.
- `GET /api/v1/characters/{id}/wishlist/addon-export` returns WL2 for the owner and RAIDASSISTENT and up,
  like the list:
  - header `WL2;<character>;<exportUnix>;<EP balance>`
  - lines `<itemID>;<ms|os>;<enhanced 0/1>;<rank>;<note>`, in list order (highest priority first,
    ranked from 1).
- The addon rolls `1+X..100+X` with X = min(balance, 50) on enhanced wishes and lowers its local
  balance after an enhanced win until the next export. It never chooses another amount (ADR 023).
- Wish JSON now uses snake_case field names, as every other endpoint and the frontend types expect.

## Nachtrag: Entzaubern und Gildenbank

RaidLedger 1.6 kann Items, die niemand will, entzaubern lassen oder für die Gildenbank behalten.
RS2 kennt dafür die Zustände `disenchanted` und `bank`; der Empfänger ist, wer das Item hat (der
Entzauberer oder der Bankcharakter). Solche Zeilen sind kein Loot eines Spielers: die Vorschau zeigt sie
mit Hinweis, sie werden nie übernommen (`not_importable`), und eine früher übernommene Zeile, die im
Addon später so markiert wurde, ändert sich nur über die Loot-Korrektur.

## Nachtrag: RaidLedger ersetzt RaidSync

RaidLedger übernimmt auch Einladungen, Gruppensortierung und den Teilnahme-Export von RaidSync;
RaidSync wird nicht mehr verwendet. Alle Formate sind jetzt `;`-getrennt (RS2):

- Einladungs-Export (Webseite → Addon): `RS2;INV;<raidID>;<raidName>;<raidSize>`, dann je Zeile
  `<name>;<TANK|HEAL|DD>;<class>;<bench 0/1>`. Freitext ersetzt `;` durch `,` und `|` durch `/`.
- Teilnahme-Export (Addon → Webseite): `RS2;ROSTER;<raidID>;<recorder>;<exportUnix>`, dann je Zeile
  `<name>;<classToken or ->;<firstSeenUnix>`. Der Import liest weiterhin auch RS1 (`|`) von RaidSync.
- Der Loot-Import liest RS1 und RS2 wie bisher.

## Consequences

RaidSync (RS1) keeps working unchanged. A new export field or state needs a new version prefix and
matching changes in both repositories. The website does not withdraw or move awards because of an
addon export; corrections stay audited staff actions.
