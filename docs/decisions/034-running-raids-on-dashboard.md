# ADR 034: Keep Running Raids on the Dashboard

## Decision
At the owner's request, reaching a raid's scheduled start changes its dashboard
presentation to `Läuft` rather than removing it from the dashboard. A scheduled,
unarchived PLANNED or OPEN raid is running when its start is at or before the
current time. The display updates while the dashboard remains open.

The banner selects the earliest scheduled active raid, retaining the existing
start-time ordering and ID tie-breaker. Running raids therefore precede future
raids. The banner heading becomes `Laufender Raid`; the agenda also retains
running raids with the `Läuft` label. Raids without a valid scheduled start,
cancelled raids, completed raids and archived raids are excluded from this
active selection. Archived raids remain in history.

Raid staff ends the raid explicitly through the existing `Archivieren` action
in ADR 021, including its permission checks, EP processing, lock expiry and
successor scheduling. `Läuft` is a time-derived dashboard label, not an additional
persisted lifecycle status or an automatic finalization operation.

## Verification
Frontend regressions cover initial loading of already-started PLANNED and OPEN
raids, crossing the exact start while the dashboard is open, continued visibility
after that start, timer cleanup, and selection of future raids when earlier raids
are completed. Existing archival tests cover the explicit finalization action.
