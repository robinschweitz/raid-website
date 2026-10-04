# ADR 025: Reviewed Local WotLK Loot Catalog

## Status
Superseded by ADR 027 (`027-bundled-loot-catalog-normalized-modes.md`). The dataset, provenance,
and licensing rules below still apply; the Admin upload, preview, and merge flow no longer exists.

## Decision
The owner selected imported boss/item tables for original WotLK 3.3.5, rather than manually
maintaining every dropdown. Investigate AzerothCore world data as the primary source,
including item names, creature loot, chest loot, and reference loot. Do not use Wrath
Classic-only changes or infer source relationships from item names.

Use a version-pinned normalized dataset with source metadata, attribution, and reviewed
licensing. Imported tables live locally; routine loot recording never depends on remote
scraping. Import is Admin-only, preview-first, reported, transactional, and never silently
overwrites existing catalog records. Scope covers ICC, Trial of the Crusader, Ulduar,
Naxxramas, Ruby Sanctum, and Vault of Archavon, with raid-size and source difficulty data.

No import fabricates guild history, boss kills, or progression. Existing raids and free-text
historical loot retain their stored identities; catalog associations are explicit.
