# ADR 022: Preserve the Roster Actually Played

## Decision
The owner requires the raid roster to be managed from raid creation until final archiving,
with the character name, role, class, and spec actually played. These are per-raid facts,
not a live view of the character's current name or specialization.

Attendance persists `character_name`, `role`, `class`, and `spec`. New manual entries can
default name/class/spec from the referenced character, but the actual raid spec is editable.
Changing the referenced character refreshes defaults unless explicit raid values are
provided. Unrelated edits preserve existing snapshots. Edits remain actor/reason audited.
Final archive requires complete recognized active roster details and then freezes them.

Raid Archive imports use that source row's own name, role, class, and spec instead of
substituting values from the Charaktere tab or current character record. Unknown historical
source values remain visible with warnings, including incremental audited additions.
Source metadata differences on replay are reported, never silently overwritten.

Migration 005 adds empty snapshot fields to older attendance. It deliberately does not
backfill current live-character data as if it were historically accurate. Missing history
is displayed as unavailable. Explicit audited corrections may supply known values for
unarchived records; archive validation does not invent them.

## Consequences
Renaming or respeccing a character cannot rewrite an earlier raid's roster. The UI displays
saved names instead of character IDs and shows class, actual role, and actual spec. Late
character responses must not overwrite user-entered roster details. Corrections omit
unchanged unknown historical fields so unrelated edits can preserve those facts.
