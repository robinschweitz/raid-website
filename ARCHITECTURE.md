# Architecture

Modular monolith. Domains: players/characters, raids/attendance, loot, enhancepoints (ledger), tokens (ledger), roster (configurable ranks/groups), audit, legacyimport/export, httpapi (v1). `raidcompletion` and `raidcorrection` coordinate cross-domain writes through repository-owned transactions.

`raidseries` owns explicit recurring schedule configuration and local-calendar calculations.
`raidsignup` owns future-raid signup availability, player-owned character/spec alternatives,
and atomic staff acceptance into attendance through `raidcorrection` (ADR 033).
`raidcompletion` implements final archival: lifecycle, EP, lock expiry, and successor linkage
share a transaction. Attendance stores per-raid name/class/spec snapshots, not live-character
presentation data. Archived raids remain visible history (ADR 021-022). Raid staff may correct
archived roster and loot with a reason through `raidcorrection`; finalization stays fixed.
Unarchived edits and live-character corrections remain audited without requiring a reason.
Frontend editors are rendered inside the selected record (ADR 032).
`raidcorrection` also owns atomic raid quick-add: staff may create a character and optional
credential-free player together with audited attendance. Persisted retry receipts prevent
duplicate onboarding, and the frontend selects existing characters directly (ADR 037).

`legacyimport` permits provenance-checked, audited additions to archived historical imports.
It imports the first roster block in `ICC25 Raidsetup` A–F and uses explicit administrator
schedule input to bootstrap one PLANNED raid and series. Source/sheet setup mappings and
configuration-bound previews prevent duplicate bootstrap or silent replacement (ADR 029).

`loot` records outside MAIN/SECOND/ENHANCED/BOE outcomes and coordinates linked enhanced
EP costs; `raidcorrection` appends coupled compensation and audit snapshots. EP is a
read-only player-facing ledger, never an in-app roll generator (ADR 023-024).
`lootcatalog` embeds the committed source JSON as the only catalog authority, mirrors it into the
foreign-key tables at startup, and normalizes source paths into explicit NORMAL/HEROIC/HARDMODE
modes; there is no runtime catalog write. `cmd/catalogexport` is a separate source-data preparation
tool, not a runtime dependency of raid operations (ADR 025, ADR 027). The SVG/CSS design uses only
supported data (ADR 026).

Principles:
- Domain-first, boring/explicit/predictable
- Append-only ledgers for EP/tokens/audit
- Archive by default, explicit delete only (Admin)
- Edits with audit trail
- UTC storage, client TZ display
- Server-side authz
