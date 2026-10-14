# ADR 060: Sync-only legacy import API

Status: Accepted by owner, 2026-10-14.

## Decision

- `POST /api/v1/sync/dry-run` and `POST /api/v1/sync` are ADMIN-only legacy-sheet synchronization endpoints.
- Sync accepts only raid keys that already have immutable source, sheet, and raid mappings. It never creates players, characters, raids, or current-setup records, and it does not import opening EP balances or classify/archive historical raids.
- Missing links and identities are returned in the normal report as stable `sync_unmapped_raid`, `sync_unmapped_player`, and `sync_unmapped_character` errors. A report with errors performs no domain writes.
- Sync fetches, checks the preview hash, plans, reviews, validates mappings, and applies changes in one transaction. A source change returns the existing `source_changed` API error.
- Historical bootstrap remains exclusively on `/api/v1/imports`.
- When an explicitly mapped `PLANNED` or `OPEN` raid key is present in `Raid Archive` and that snapshot contains synced loot for it, sync finalizes the raid through the ordinary completion service. The normal EP awards, lock expiry, and recurring successor are therefore preserved and idempotent.
- The mapped active raid must already have both `series_id` and `scheduled_start`; sync never infers a schedule. After completion, sync maps the successor to the numeric next sheet key and reconciles `ICC25 Raidsetup` into that planned successor using only existing mapped players and characters. It adds missing attendance and corrects matching snapshot metadata; it never creates identities or silently removes other attendance.
- Sync commits sheet loot before calling the completion service, which owns its own transaction. Completion, successor mapping, and roster reconciliation are replay-safe stages; the merged source hash is recorded only after all stages succeed.
