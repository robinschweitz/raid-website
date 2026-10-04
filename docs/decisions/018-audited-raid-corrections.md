# ADR 018: Audited Raid Corrections

## Decision
The owner selected explicit audited corrections rather than freezing completed raids.
Attendance and loot PATCH operations require an actor, a nonblank reason, and before/after
audit snapshots. They are permitted on PLANNED, OPEN, and COMPLETED raids. Additions to
COMPLETED raids are corrections too and require a reason. CANCELLED and archived raids
reject attendance and loot writes.

Corrections atomically commit the record, audit entry, and all ledger consequences.
For completed raids, compute each player's eligibility before and after the correction
and append only the difference: +10 when newly eligible, -10 when no longer eligible,
and no entry when unchanged. Bench and substitute participation remain equivalent.

The owner delegated the historical EP policy to implementation. Applying only the change
in eligibility preserves imported opening balances without backfilling historical rewards.
This same rule applies to ordinary completed raids. Repeated unchanged corrections do
not award additional points. Corrections may produce a negative balance if previously
awarded points have already been spent; history is not rewritten to conceal that debt.

Token consequences are reconciled with compensating transactions linked to the award.
Corrections preserve historical lock consumption and never implicitly revive or consume
another lock. There is no DELETE endpoint or record-removal workflow in this change.

## Consequences
RAIDASSISTENT and higher may perform corrections; the server enforces this boundary.
Character/player ownership is validated. Existing award and ledger history is retained,
and all steps roll back together if audit or ledger persistence fails.
