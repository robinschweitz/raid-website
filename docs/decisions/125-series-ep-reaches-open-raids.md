# ADR 125 — A series' EP setting reaches its open raids

- Status: accepted
- Date: 2026-11-23
- Decided by: owner (2026-11-23)
- Amends: ADR 113 (point 3: "Changing a series does not change existing raids")

## Context

The series "ICC25 HC Trys" was created with the ICC pool and +10 EP; saving it planned raids 43
and 44 with those values. Six days later the owner switched the series to "Keine EP". Under
ADR 113 the planned raids kept +10, so archiving raid 43 on 2026-11-21 gave 19 players 10 EP
each, which the owner did not want. The rule was stated in the form, but it is easy to miss and
almost never what staff mean when they change a series.

## Decision

1. Changing a series' EP pool or award gives every PLANNED or OPEN, unarchived raid of the series
   the new values in the same transaction, with an audit entry per raid (`series_ep`).
2. A raid that already has EP transactions or item locks keeps its values (its pool is fixed, as
   in ADR 113); the save answer names these raids so staff can correct them by hand.
3. Completed, cancelled, and archived raids never change.
4. One-off correction of 2026-11-23: the 19 `award_no_mainspec_loot` rows of raid 43 were
   deleted on the server by the operator on the owner's instruction, with one audit entry per
   deleted row holding its full content, after a verified snapshot
   (`raid.db-20261123T152747Z`). Raids 43 and 44 were set to no pool and award 0. This is a
   documented exception to FR-16.1 and FR-31.2 (the owner did not want correction entries in
   the players' EP history); it does not change those rules for the future.

## Consequences

- A raid planned from a series follows later EP changes of the series until it gets EP or locks.
- A raid whose EP was chosen by hand on the new-raid form (FR-10.1) also takes a later series
  change; staff who want an exception change the series after the raid has points, or not at all.
