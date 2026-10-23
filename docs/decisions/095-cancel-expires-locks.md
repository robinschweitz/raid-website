# ADR 095: Cancelling a raid expires its unused item locks

Status: Accepted.

## Context

ADR 012 and ADR 021 expire unused item locks when a raid is archived. Cancelling a raid only set the status, so its locks stayed active. The owner decided on 2026-10-22 (OQ-2) that cancelling must expire them too.

## Decision

This ADR amends ADR 015 and ADR 021 for the cancel path only.

- `raid.Service.CancelRaid` expires every active lock of the raid in the same transaction as the status change. The expiry costs no EP.
- The raid cancel audit entry and one audit entry per expired lock are written in that transaction. If any step fails, the raid stays uncancelled.
- Locks of raids cancelled before this change are not touched; there is no backfill (FR-17.7).
