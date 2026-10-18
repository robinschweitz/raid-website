# ADR 078: Plan raids of a series ahead

Status: Accepted. Amends ADR 021 and ADR 033.

## Context

Raids of a series were created one at a time: by hand, or as the single successor when the current
raid was archived (ADR 021). Players could only sign up for the next raid, and signups had to be
opened per raid (ADR 033).

## Decision

The owner asked for raids of a series to be scheduled into the future so people can sign up in
advance, with both a manual and an automatic way, no fixed limit on how far ahead, signups open right
away, and the feature available to `RAIDASSISTENT` and higher.

1. **Plan up to a date (manual).** `POST /api/v1/raid-series/{id}/plan` with `{"until": "YYYY-MM-DD"}`
   (`RAIDASSISTENT`+) creates every missing occurrence of the series after now up to the end of that
   day in the series timezone. The Raids page offers it as "Termine im Voraus anlegen".
2. **Rolling window (automatic).** A series has `plan_weeks_ahead` (Admin-configured, `0` = off). The
   window is topped up whenever the series is saved and whenever a raid of the series is archived.
   There is no background job.
3. **How occurrences are chosen.** Planning continues from the latest raid of the series, so
   multi-week intervals keep their rhythm; elapsed occurrences are skipped. Existing raids are never
   changed. Names come from the series' name template and counter, leader, instance and size from the
   series. Each created raid is audited as `raid`/`plan`.
4. **Signups open right away** on every raid the site creates from a series: planned raids and the
   successor created on archive. This replaces ADR 033's "recurrence successors start disabled".
5. **Cancelled occurrences are skipped when archiving.** ADR 021 treated a cancelled occurrence as a
   conflict that blocked archiving. With raids planned ahead, cancelling one week (a holiday) is
   normal, so archiving now skips cancelled occurrences and links or creates the next one in the
   same rhythm. Completed, archived or differently linked occurrences still conflict.
6. **Runaway guard.** Planning further than 520 weeks (ten years) ahead, or a window larger than
   that, is rejected. This only protects against typos; it is not a business limit.

## Consequences

Players can see and sign up for raids weeks ahead. Raid leads cancel single future raids instead of
deleting them; archiving moves past them. Existing series keep `plan_weeks_ahead = 0`, so nothing is
created until someone plans or sets a window.
