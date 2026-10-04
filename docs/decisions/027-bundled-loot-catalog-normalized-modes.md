# ADR 027: Bundled File-Authoritative Loot Catalog With Normalized Modes

## Status
Accepted. Supersedes ADR 025.

## Context
ADR 025 made the catalog an Admin-uploaded, preview-first import. The source-derived dataset
`scripts/data/catalog-wotlk335.json` already exists in the repository, is pinned, licensed, and
partially coverage-checked. Runtime upload therefore only added a path by which a bundled,
reviewed file could be replaced by hand, and a mode selector that had to offer raw source paths
(`difficulty` / `loot_mode`) that only the exporter understood.

## Decision
The JSON file is the single authority for catalog reads. It is embedded with `go:embed`
(`scripts/data/catalog.go`) and mirrored into the foreign-key tables at startup so historical
awards keep valid references. Startup fails on an invalid source or a failed mirror; there is no
runtime catalog write, upload, preview, or merge endpoint.

Source paths are normalized once, server-side, into explicit modes: `NORMAL` for
`raid-difficulty:0/1`, `HEROIC` for `raid-difficulty:2/3`, and `HARDMODE` for Ulduar
`hardmode:true`, which stays conditional. Loot masks and reference paths never imply a mode.
Relationships sharing `[boss_id, item_id, raid_size, mode]` are merged, their raw
`difficulty`/`loot_mode` pairs are kept as `source_variants`, and an unproven condition marks the
drop `conditional`. Unknown structured conditions are treated conservatively as conditional.

New loot may only reference a bundled boss/item pair for the raid's persisted
`catalog_instance_id` and `raid_size`; names are derived server-side. Missing context is
`catalog_context_required` (409), an invalid pair is `catalog_item_invalid` (400).

## Consequences
- The catalog cannot diverge from the committed file at runtime, and the UI is read-only.
- The database mirror is a reference target, not a read model; it retains historical rows and never
  deletes them, and each startup records an initialization report in `loot_catalog_initializations`.
- Updating the catalog means committing a new file, not importing data.
- Existing free-text and imported historical loot is untouched and cannot be newly created.