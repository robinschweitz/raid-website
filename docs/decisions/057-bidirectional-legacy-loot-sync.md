# ADR 057: Bidirectional legacy loot sync

Status: Accepted by owner, 2026-10-13.

## Decision

- The legacy loot `Import ID` is authoritative for bidirectional synchronization.
  A source row with the same Import ID identifies the same website loot award.
- A changed source row is an audited correction, not a new independent award. Source
  omissions withdraw the linked award; a later reappearance restores it. Withdrawal
  preserves the award and its history rather than deleting it.
- A legacy source raid may be explicitly linked to an existing website raid. Such a
  link and all synchronization state and events are append-only.
- The eventual review/apply workflow performs the audited correction, withdrawal, and
  restoration consequences atomically, including their existing EP effects.

## Consequences

Migration 057 adds withdrawal fields to loot awards and append-only persistence for
raid links, source snapshots, and award synchronization events. It does not change
legacy import review/apply behavior or expose an API.

This amends ADR 019's treatment of changed Import IDs: once bidirectional sync is
enabled, a changed authoritative source row is corrected with audit history instead
of being rejected as an overwrite conflict.
