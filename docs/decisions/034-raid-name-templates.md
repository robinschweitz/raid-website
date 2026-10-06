# ADR 034: Configurable Raid Name Templates

## Decision
Raid series support a configurable naming template with variables. When a successor raid is
created during archival, the template is evaluated with the series' auto-incrementing counter
and the raid's scheduled date. The counter increments after each successor creation.

Supported template variables:
- `{number}` — the auto-incrementing counter (e.g., `34`)
- `{name}` — the series base name (e.g., `icc25`)
- `{date}` — the raid date in `YYYY-MM-DD` format
- `{week}` — the ISO week number

The template and counter are stored on the `raid_series` row. When the template is empty,
the successor falls back to the series name (preserving pre-existing behavior).

## Migration
Existing series are auto-configured at startup by detecting the `prefix-NUMBER` pattern in
their raid names. For example, a series with raids named `icc25-01` through `icc25-33` gets
`name_template = "icc25-{number}"` and `next_raid_number = 34`. Series without a detectable
pattern keep their current name as the template with a counter of 1.
Detection only runs for an untouched template (empty or equal to the series name); an
explicitly configured template or counter is never overwritten by a restart.

## Manual raid creation
A manually created raid may omit the name when a series is selected: the server derives the
name from the series template with its next counter (incremented in the same transaction) or
from the series name when the template is empty. An explicit name is stored as given and does
not consume a counter value. A raid without a series requires an explicit name. The series
selection itself is optional when creating a raid.

## API
The series request accepts `name_template` and `next_raid_number` fields. Both are optional
on create; on update they can be changed independently of other configuration. Creating a
raid without a name and without a series is rejected with `name_required`.

## Frontend
The series editor shows a template input with a hint listing available variables. The next
counter value is editable. The series list displays the template and next number.
The raid creation form offers an explicit "Keine Serie" option, requires the name only when
no series is selected, and previews the derived name from the template while the name field
is empty.
