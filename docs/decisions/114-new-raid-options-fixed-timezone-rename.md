# ADR 114 — New-raid options, one fixed timezone, editable raid names

- Status: accepted
- Date: 2026-11-13
- Decided by: owner (2026-11-13)
- Amends: ADR 021 (IANA timezone per series), ADR 035 (series name ignores a sent name), ADR 112, ADR 113

## Context

Staff create single raids from the "Neuer Raid" form. It only offered one raid leader, and the
EP pool, the EP award, and the name came from the series without a way to change them for one
raid. Fixing such a raid afterwards was not possible for the name at all (FR-10.5). The series
form asked for a free-text IANA timezone, although the guild plays on one realm whose server
time is German time.

## Decision

1. **New-raid form.** Creating a raid may send all raid leaders in order (`leader_ids`, which
   wins over `raid_leader_id`), an EP pool (`ep_pool_id`, 0 = no pool), and an EP award
   (`ep_award`, 0 to 100). Values that are not sent come from the series as before, or the
   defaults without a series. The form prefills them from the chosen series.
2. **Name with a series.** A sent name wins over the series' name template. A raid created
   from a series still consumes the template counter, so numbering stays in step.
3. **Renaming.** Staff can change the name of every PLANNED or OPEN, unarchived raid, also of
   a series raid. Archived raids stay unchanged by this rule. The `name_not_editable` error is
   gone.
4. **One timezone.** Every raid series uses `Europe/Berlin` (the realm's server time). The
   series form has no timezone field; the API rejects any other sent value with
   `422 invalid_series`. Migration 090 sets every stored series to `Europe/Berlin`. The
   `timezone` field stays in series responses for the addon export.

## Consequences

- Existing series with another timezone shift to German time at migration 090.
- The audit log records the leaders sent on creation as a `set_leaders` entry next to `create`.
