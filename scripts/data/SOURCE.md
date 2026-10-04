# WotLK 3.3.5 Catalog Source Record

## Status

`catalog-wotlk335.json` is a usable **PARTIAL, SOURCE-CHECKED dataset**, not a complete
catalog and not a human-reviewed release. It is embedded with `go:embed` and served as the
only catalog; the server mirrors it at startup. Do not advertise it as a complete catalog or
imply complete coverage. The first notices state this limitation in German.

Outputs generated on 2026-10-04:

- `catalog-wotlk335.json`: 6 instances, 97 real creature-ID encounter participants,
  2,596 distinct items, 4,330 boss/item/size/source-mode relationships.
- `catalog-wotlk335.json.LICENSE`: verbatim, separate upstream GNU GPL v2 license.
- `catalog-wotlk335.json.coverage.json`: actual per-instance/size/difficulty counts,
  distinct item counts, participants with drops, loot modes, and `partial: true`.
- `catalog-wotlk335.manifest.json`: selected encounter enum symbols, source script names,
  explicit chest roots, executable source assertions, and scope notices.

Canonical dataset hash:
`5eb7c086ba61d5c27c79c6da925fbc85292b6e866509a8f2e91b98a4bf36bb26`.
This is the exporter's sorted-JSON SHA-256. The API reports the embedded file's byte hash in
`raw_hash`, which is a different value.

Pinned AzerothCore revision:
[`ca7e5015d01e8818cb20b1265185f73a691e273f`](https://github.com/azerothcore/azerothcore-wotlk/tree/ca7e5015d01e8818cb20b1265185f73a691e273f).
Every source path below is relative to that exact revision, never a moving branch.
Every fetched source file used for extraction/proofs has its SHA-256 recorded in notices.
Upstream AUTHORS is retained verbatim in notices. No raw world SQL is stored in this repo.

## Coverage

Counts are **drop relationships**, not kills, raid history, item roll chances, or guild
statistics. Participant metadata includes genuine multi-participant encounters and
minibosses; 97 participants must not be presented as 97 distinct encounters.

| Instance | Map | Participants | 10 Normal | 25 Normal | 10 Heroic | 25 Heroic |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| Icecrown Citadel | 631 | 16 | 132 | 158 | 154 | 171 |
| Trial of the Crusader | 649 | 36 | 116 | 322 | 180 | 322 |
| Ulduar | 603 | 19 | 238 + 18 + 30 | 532 + 62 + 76 | not invented | not invented |
| Naxxramas | 533 | 18 | 270 | 481 | not invented | not invented |
| Ruby Sanctum | 724 | 4 | 16 | 20 | 13 | 17 |
| Vault of Archavon | 624 | 4 | 498 | 504 | not invented | not invented |

Ulduar's sums distinguish opaque/mixed source modes plus explicitly proven normal-chest
and hardmode-chest categories for Mimiron and Thorim. Ulduar hardmodes are **not** separate
heroic raid difficulties. Details and distinct item counts are in the coverage JSON.

Ruby Sanctum heroic records cover Halion. Its three minibosses have no explicit heroic
DifficultyEntry rows. The exporter does not clone normal drops into missing modes.
Likewise, Sister Svalna is present as ICC metadata and source normal variants, without
fabricating absent heroic variants. Creature.cpp does implement runtime fallback, but
this dataset deliberately records only explicitly present difficulty variants, not a
synthetic expansion into every runtime fallback case.

## Proven Extraction

All instance IDs are checked against `data/sql/base/db_world/instance_template.sql` and
their corresponding script keys. Display names are source-checked conventional instance
labels supplied by the manifest; boss names and item IDs/names come from source tables.

Selected primary encounter/participant symbols are resolved from these examined headers:

- `src/server/scripts/Northrend/IcecrownCitadel/icecrown_citadel.h`.
- `src/server/scripts/Northrend/CrusadersColiseum/TrialOfTheCrusader/trial_of_the_crusader.h`.
- `src/server/scripts/Northrend/Ulduar/Ulduar/ulduar.h`.
- `src/server/scripts/Northrend/Naxxramas/naxxramas.h`.
- `src/server/scripts/Northrend/ChamberOfAspects/RubySanctum/ruby_sanctum.h`.
- `src/server/scripts/Northrend/VaultOfArchavon/vault_of_archavon.h`.

The Naxxramas header omits several numeric boss IDs. Those seven IDs are resolved from
the unique `creature_template.ScriptName`, with the corresponding source file checked:
`boss_anubrekhan.cpp`, `boss_faerlina.cpp`, `boss_maexxna.cpp`, `boss_grobbulus.cpp`,
`boss_gluth.cpp`, `boss_noth.cpp`, `boss_loatheb.cpp`, under the Naxxramas directory.

`src/server/shared/DataStores/DBCEnums.h` explicitly defines difficulty 0/1/2/3 as
10-normal/25-normal/10-heroic/25-heroic. `Creature.cpp` selects
`normalInfo->DifficultyEntry[diff - 1]`. The exporter checks those definitions, resolves
actual variant creature rows, and follows each variant's actual `LootID`; it never assumes
`LootID == creature entry`. A zero/missing loot root is reported, not replaced by another
participant's loot. Reference loot is expanded recursively with cycle/missing checks.

The exporter retains complete source lootmode paths in `difficulty`, not an inferred
normal/hardmode label. German `item_template_locale` names take precedence; fallback to
the source item name is individually reported. No roll chances or kill state are inferred.

### Verified Chest Roots

| Encounter | Source GO entries | Verified selection |
| --- | --- | --- |
| Deathbringer Saurfang | 202239 / 202240 / 202238 / 202241 | 10N / 25N / 10H / 25H |
| Kologarn | 195046 / 195047 | `RAID_MODE` 10 / 25 |
| Algalon | 194821 / 194822 | `RAID_MODE` 10 / 25 |
| Mimiron | 194789 / 194956 | normal chest, 10 / 25 |
| Mimiron | 194957 / 194958 | `_hardmode` chest, 10 / 25 |
| Thorim | 194312 / 194314 | normal chest, 10 / 25 |
| Thorim | 194313 / 194315 | source code adds 1 to the selected chest ID for `_hardMode` |

The chest entry definitions and actual `SummonGameObject` selections are checked as
regex assertions against the examined headers and `boss_kologarn.cpp`,
`boss_algalon_the_observer.cpp`, `boss_mimiron.cpp`, and `boss_thorim.cpp`.
`src/server/game/AI/ScriptedAI/ScriptedCreature.h` proves that the two-argument
`RAID_MODE` returns its first argument for difficulty 0 and second for difficulty 1.

Saurfang's `instance_icecrown_citadel.cpp` stores these four entries in
`DeathbringersCacheGUID`, unlocks that cache on `DATA_DEATHBRINGER_SAURFANG == DONE`,
and assigns `SetLootRecipient(deathbringer)`. `gameobject.sql` independently proves all
four static map-631 spawns with spawnMask 1/2/4/8. The exporter checks those map/difficulty
spawns rather than trusting a GO constant's suffix alone.

Every chest is then checked as `gameobject_template.type == 3`; its actual `Data1` is
followed into `gameobject_loot_template`, including reference chains. Those are data
relationships, not relationships inferred from similarly named items or chests.

## Concrete Missing Cases

### ICC Gunship Battle

Examined `icecrown_citadel.h` and `instance_icecrown_citadel.cpp`. The script groups all
eight Armory constants into `GunshipArmoryGUID` and unlocks that object when
`DATA_ICECROWN_GUNSHIP_BATTLE == DONE`; faction filtering is explicit. Entries are
Horde 202178/202180/202177/202179 and Alliance 201873/201874/201872/201875.
However, it assigns the recipient to the **instance**, not either captain's creature.
The current DTO has `boss_id` as a real creature ID and no collective encounter ID.
Assigning this shared chest to Saurfang 36939 or Muradin 36948 would invent a mapping
policy. Both captains remain metadata; no Armory drops are assigned to them.

### ICC Valithria Dreamwalker

The header defines Cache entries 201959/202339/202338/202340, and the instance script
assigns a created cache's recipient to Valithria. `boss_valithria_dreamwalker.cpp`
declares Dream Slip 71196 and Spawn Chest 71207. The four cache entries have **no
static rows** in the examined pinned `gameobject.sql` or `gameobject_summon_groups.sql`.
The inspected `spell_dbc.sql` base table lacks the relevant client spell-effect records
needed to prove which chest entry Spawn Chest creates in each mode. Header suffixes
and recipient assignment alone do not prove the dynamic spell-effect selection.
All four cache modes are omitted; Valithria 36789 remains metadata.

### Ulduar Freya

`boss_freya.cpp` explicitly casts an elder-count-indexed chest spell matrix:
10-player 62957/62955/62953/62950, 25-player 62958/62956/62954/62952.
Those eight spell rows are absent from the pinned base `spell_dbc.sql`. Without their
client spell-effect GO targets, the exact chest entry and reference-loot root for each
elder count cannot be derived from the fetched world tables. No chest ID is guessed.
Freya 32906 and the three elders remain metadata; Freya chest drops are absent.

### Ulduar Hodir

`ulduar.h` defines normal chests 194307/194308 and rare chests 194200/194201.
`instance_ulduar.cpp` has `GetHodirChest`, unlocks chests through
`setChestsLootable(BOSS_HODIR)`, and hides the rare cache when hardmode is missed.
`boss_hodir.cpp` drives the rare-cache timer. Nevertheless these four entries have no
static rows in the examined pinned `gameobject.sql` or `gameobject_summon_groups.sql`.
The actual creation/raid-size selection path is not established by the examined files;
`GetHodirChest` merely chooses whichever GUID is present. The exporter does not turn
those names or that fallback lookup into a fabricated spawn mapping. All four chest
variants are omitted; Hodir 32845 remains metadata.

### Naxxramas Four Horsemen

The source fully proves entries 181366 (10) and 193426 (25). In
`boss_four_horsemen.cpp`, the last participant to die summons that chest when the
collective `BOSS_HORSEMAN` state is done and calls `SetLootRecipient(me)`.
The actual recipient creature varies with kill order. There is no single authoritative
creature ID for this collective chest. Assigning it arbitrarily to Rivendare, Zeliek,
Blaumeux, or Korth'azz would fabricate a fixed mapping; multiplying the shared chest
across all four participants would imply four independent loot sources. The four real
IDs remain metadata without chest drops, pending an explicit collective-encounter policy.

### ToC Faction Champions

`instance_trial_of_the_crusader.cpp` explicitly selects Crusaders' Cache entries
195631/195632/195633/195635 by raid difficulty, after
`INSTANCE_PROGRESS_FACTION_CHAMPIONS_DEAD`, then sets the recipient to the instance.
It is one shared encounter cache, not loot owned by a particular one of the 28 champion
creatures. The DTO has no collective encounter key. All 28 champion participants remain
metadata; none receives a fabricated independent cache relationship.

### ToC Tribute Chests

The same script proves eight heroic Tribute entries, chosen by `AttemptsLeft`:
10-player 195668/195667/195666/195665 and 25-player 195672/195671/195670/195669.
The thresholds are explicit source conditions, but these are raid/attempt-count rewards,
not an ordinary single-creature drop source. The current drop DTO lacks that encounter
and attempt-condition identity. Assigning all tribute variants to Anub'arak 34564 would
misrepresent them as his unconditional boss drops. All eight are omitted.

### Scripted Corpse Hardmode Interpretation

The source has real conditional loot-mode mutations: Leviathan adds `1 << _towersCount`,
XT-002 adds `LOOT_MODE_HARD_MODE_1`, Vezax switches 1 to 3, Yogg-Saron starts at 31 and
removes/adds watcher-related bits, and the Iron Council suppresses loot until the last
member. Relevant source files were examined, but these state machines are not fully
translated into source-condition labels. All existing SQL lootmode/path values remain
opaque and visible. Do not label every retained Ulduar mode as unconditional normal loot
or provide an inferred universal hardmode selector. Multi-participant corpse roots remain
attached only to creatures whose own source LootID contains those relationships.

## Licensing And Reproduction

The pinned root LICENSE is GNU GPL v2, not AGPL. Examined world SQL headers are MySQL
dump headers without a separate SQL-specific license grant; the examined C++ headers
identify the AzerothCore project, refer to AUTHORS, and state GPL v2 or later.
The derived data is delivered separately with the verbatim upstream license, attribution,
modification date, source paths, and source hashes. This is not an application relicense
or a claim of application-license compatibility. No upstream implementation is copied
into the exporter; regex assertions test the pinned sources.

Reproduce to a new output path; the exporter refuses to overwrite any artifact:

```powershell
Test-Path -LiteralPath "C:\Users\Robin\AppData\Local\Temp\opencode"
go run ./cmd/catalogexport -manifest scripts/data/catalog-wotlk335.manifest.json -out "C:\Users\Robin\AppData\Local\Temp\opencode\catalog-wotlk335.json"
```

The command is the only networking path. The application serves the committed JSON locally and
never fetches at runtime. World SQL is parsed lexically, never executed. No dependency was added.
Offline exporter/data tests validate source assertions, source variant sizing, static
spawn masks, missing-root notices, catalog links/duplicates, the generated hash, counts,
coverage totals, excluded unverified chest associations, and the separate license.
