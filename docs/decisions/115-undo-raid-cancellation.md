# ADR 115 — Taking back a raid cancellation

- Status: accepted
- Date: 2026-11-14
- Decided by: owner (2026-11-14)
- Amends: ADR 015 (cancelled is final)

## Context

A raid that was cancelled by mistake, or that takes place after all, could only be replaced
by a new raid. For a series raid that also breaks the chain of occurrences.

## Decision

1. **Undo.** Staff (RAIDASSISTENT and higher) can take back the cancellation of any cancelled,
   unarchived raid, also after its scheduled start: `POST /api/v1/raids/{id}/uncancel`.
   The UI asks for confirmation first.
2. **Status.** The raid gets the status it had before the cancellation, taken from the
   `before` snapshot of the newest `cancel` audit entry (PLANNED or OPEN). Without such an entry
   (a raid cancelled before audit coverage), it becomes PLANNED. The cancellation time is cleared.
3. **Locks.** Item locks that the cancellation expired (FR-17.6) stay expired; players set them
   again. Nothing else is restored: EP, signups, roster, and loot were not changed by the
   cancellation.
4. **Audit.** The undo writes the audit entry `uncancel` with before and after.

## Consequences

- A non-cancelled or archived raid answers `409 raid_not_cancelled`.
