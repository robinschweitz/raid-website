# ADR 062: Automatic Google Sheets sync

Status: Accepted by owner, 2026-10-14.

Supersedes ADR 060's manual-only application requirement.

## Decision

- When `GOOGLE_SERVICE_ACCOUNT_FILE` is configured, the server starts a backend-only Google Sheets synchronization every minute after HTTP initialization.
- Each run first delivers pending website-to-sheet changes. Delivery failures stop that interval's incoming work. It then enumerates distinct `google_sheets` sheet IDs from adopted `legacy_sync_raid_mappings`, resolves an active stored `ADMIN` account as actor, and runs `DryRunSync` followed by `MergeSync` only when the preview has zero errors.
- Automated merges use the fixed audit reason `Automatische Google-Sheets-Synchronisierung`. No request header or HTTP endpoint supplies the actor.
- Runs are serialized. Preview, merge, mapping, and actor failures are logged; a failed or skipped sheet does not stop later sheets or future intervals.

## Consequences

Only explicitly adopted mappings are synchronized automatically. Historical bootstrap remains manual, and an unconfigured Google service account starts no scheduler.
