# Catalog Integration Handoff

Implemented under ADR 025 and ADR 027. The production router, raid/series metadata, loot
validation, and the read-only frontend catalog view are integrated. This file records the
data/API contract and remaining source-coverage limits, not outstanding wiring work.

## Coordinator Wiring

`internal/httpapi/router.go` builds the catalog service and initializes the mirror before the
listener is opened:

```go
catalog := lootcatalog.NewService(lootcatalog.NewRepo(db))
report, err := catalog.EnsureBundled(context.Background())
if err != nil { return err }
v1.CatalogHandlers{Catalog: catalog}.Register(mux)
```

An invalid embedded source or a failed mirror stops startup. Initialization is idempotent and
serialized through a sentinel row on the SQLite writer lock.

Migration `007_loot_catalog` is picked up by the existing embedded migration loader.
It adds nullable `catalog_instance_id TEXT` and `raid_size INTEGER` to `raids` and
`raid_series`, plus the catalog and preview tables. It seeds no guild/game records.
Migration `009_bundled_catalog` adds `loot_catalog_initializations` for the startup report.

`CatalogInstanceID *string` and `RaidSize *int` are exposed with snake_case JSON tags on
raid/series models and requests, validated with transaction-bound catalog repositories,
and inherited by newly created successors. Legacy free-text raids keep their stored
identities, but new loot requires the association. Local drops must evidence the selected size.
Migration 008 adds optional `catalog_boss_id` to awards. New loot validates boss/item IDs
against the bundled catalog for the raid's instance and size and derives names server-side.
Unchanged historical identities remain intact. Preplanned successors are never overwritten.
The legacy sheet import decodes the sheet raid key into the catalog association for raids it
creates, and derives an imported award's boss from the bundled catalog when the item belongs to
exactly one boss (ADR 028). Unproven items keep an empty boss and are reported; item names and
other free text are never rewritten to match the catalog.

## HTTP Contract

All reads require an authenticated account, including GUEST. There are no catalog writes.

| Method | Path | Data shape |
| --- | --- | --- |
| GET | `/api/v1/loot-catalog` | `Catalog` |
| GET | `/api/v1/loot-catalog/instances` | `Instance[]` |
| GET | `/api/v1/loot-catalog/bosses?instance_id=631` | `Boss[]` |
| GET | `/api/v1/loot-catalog/items?boss_id=36612&raid_size=10&mode=HEROIC` | `ItemDrop[]` |

Read query filters are optional. `mode` is an explicit enum, never a source string; an unknown
mode is `catalog_invalid` (422) and `raid_size` must be 10 or 25. Item results are one row per
`[boss_id, item_id, raid_size, mode]`; `source_variants` keeps the raw pairs and `conditional`
marks unproven conditions.

```ts
type Source = {
  id: string; revision: string; url: string; license: string; attribution: string;
};
type Instance = { id: string; name: string };
type Boss = { id: string; instance_id: string; name: string };
type Item = { item_id: string; name: string; binding?: number; quality?: number };
type Drop = {
  boss_id: string; item_id: string; raid_size: 10 | 25;
  difficulty: string; loot_mode: number;
};
type Dataset = {
  source: Source; notices: string[];
  instances: Instance[]; bosses: Boss[]; items: Item[]; drops: Drop[];
};
type Provenance = { source: Source; raw_hash: string; notices: string[] };
type ItemDrop = Item & {
  boss_id: string; raid_size: 10 | 25; mode: 'NORMAL' | 'HEROIC' | 'HARDMODE';
  conditional: boolean; source_variants: { difficulty: string; loot_mode: number }[];
};
type Catalog = {
  sources: Provenance[];
  instances: Instance[]; bosses: Boss[]; items: Item[]; drops: ItemDrop[];
};
type Report = { source: Source; raw_hash: string; notices: string[]; errors: string[] };
```

Success is `{data: ...}`; errors are `{error: {code, message}}`. Catalog read errors are
`catalog_invalid` (422). Loot writes add `catalog_context_required` (409) when the raid has no
stored instance and size, and `catalog_item_invalid` (400) when the pair is not in the bundled
catalog for that context. No array is null.

Reads are served from the immutable embedded source, never from the mirror, so a stale or
partially updated database cannot change what the API reports. `raw_hash` is the SHA-256 of the
embedded file bytes; the exporter's canonical sorted-JSON hash is a different value.

## Data Delivery And Blocker

The committed dataset at `scripts/data/catalog-wotlk335.json` is embedded and served
automatically: six instances, 2,596 distinct items, and 4,330 sourced drop relationships. It is
still a **partial** dataset. Completing the source-derived catalog requires a reviewed manifest
of every boss variant and chest-to-encounter association, especially scripted Ulduar and
Trial of the Crusader chests/hardmodes. The exporter deliberately requires those explicit
associations rather than deriving them from item names or unverified lootmode numbers.
`scripts/data/SOURCE.md` and the coverage JSON list exact proven relationships, missing cases,
notices, licensing, and reproduction steps. Participant NPC counts are not boss-kill/progression
counts.

The reusable standard-library exporter is `cmd/catalogexport`. Example:

```powershell
Test-Path -LiteralPath "C:\Users\Robin\AppData\Local\Temp\opencode"
go run ./cmd/catalogexport -manifest internal/lootcatalog/testdata/marrowgar.manifest.json -out "C:\Users\Robin\AppData\Local\Temp\opencode\marrowgar.catalog.json"
```

The output path and its `.LICENSE` sidecar must not exist. The tool fetches only a full
commit SHA, lexically parses CREATE/INSERT data, never executes SQL, recursively expands
creature/chest reference loot with cycle/missing-reference checks, preserves complete
source mode paths in `difficulty`, uses German item locale names with reported fallback,
and includes file SHA-256 hashes and upstream AUTHORS in notices. SQL files are never
stored in the application. Outputs still require human review; a manifest evidence URL
is a review reference, not automated proof of its claim. Instance display names are
reviewed manifest labels; boss names and item IDs/names come from the pinned tables.

Included source-derived offline fixture:

- `testdata/marrowgar.manifest.json`: explicit incomplete export manifest.
- `testdata/marrowgar.catalog.json`: ICC 631, Lord Marrowgar 36612, base 10-player variant,
  13 actual items/drops including reference table 34230 and item 50759.
- `testdata/marrowgar.catalog.json.LICENSE`: verbatim upstream GPL v2 license.
- Canonical hash: `174a4d8ee52eb47ab89b457e8e778bb289370800f994c0d70bbe7c4849b80316`.

The fixture is testdata only, not embedded into the server. Its source notices prominently
state incompleteness. Exporter/parser and API/domain tests are offline.

## Source Research

Pinned upstream revision: `ca7e5015d01e8818cb20b1265185f73a691e273f` (2026-10-04).
Repository: <https://github.com/azerothcore/azerothcore-wotlk>.

At this pin, `data/sql/base/db_world/instance_template.sql` verifies these map/script keys:

| Map ID | Actual instance | Source script |
| --- | --- | --- |
| 631 | Icecrown Citadel | `instance_icecrown_citadel` |
| 649 | Trial of the Crusader | `instance_trial_of_the_crusader` |
| 603 | Ulduar | `instance_ulduar` |
| 533 | Naxxramas | `instance_naxxramas` |
| 724 | Ruby Sanctum | `instance_ruby_sanctum` |
| 624 | Vault of Archavon | `instance_vault_of_archavon` |

`src/server/shared/DataStores/DBCEnums.h` explicitly gives raid difficulty 0 as
10-player normal, 1 as 25-player normal, 2 as 10-player heroic, 3 as 25-player heroic.
`src/server/game/Entities/Creature/Creature.cpp` maps difficulty entries using `diff - 1`.
The fixture base creature 36612 and its reference 34230 were verified in the pinned
creature/loot tables; `icecrown_citadel.h` identifies NPC_LORD_MARROWGAR as 36612.
These facts do not prove all other encounters' scripted chest/hardmode associations.

The root `LICENSE` at this pin contains GNU GPL v2, not AGPL. The examined SQL files have
MySQL dump headers without SQL-specific copyright/license notices; no separate license
file exists under data/sql/base/db_world or its data/sql parents. Upstream AUTHORS and
the verbatim license are preserved with the derived fixture. The independently supplied
dataset is not a declaration of application-license compatibility or an application
relicense. No AzerothCore implementation code was copied into the exporter.
