# ADR 038: Raid roster with a single Ersatzbank and a class-locked snapshot

Status: Accepted

## Decision

The owner requested three changes to the raid roster form (`Raidaufstellung`):

1. **One participation category instead of two.** The roster form offers a single
   `Ersatzbank` checkbox instead of the former `Ersatz` and `Bank` pair. The checkbox
   writes `is_bench`; `is_substitute` is no longer offered anywhere in the UI. The
   column and API field stay in place untouched, and no stored data is rewritten.
   Roster board, roster heading, and dashboard counters label the category
   `Ersatzbank` only. The owner confirmed that no `Ersatz` or `Bank` entries were ever
   recorded, so no migration or reclassification is needed.
   The dashboard's `Bestätigt` count now means "not on the Ersatzbank", matching the
   roster heading `X im Raid · Y auf der Ersatzbank`.

2. **Class follows the selected existing character.** The `Gespeicherter
   Charaktername` input was a duplication of the character dropdown and is removed:
   the stored name comes from the selected character and is no longer typed in the
   form. `Gespeicherte Klasse` is filled in and read-only when an existing character
   is selected. Only the actual spec stays editable, plus role and the participation
   flags. The stored snapshot semantics of ADR 022 are unchanged: snapshots are still
   sent per changed field, still never blank, and still never rewritten from later
   live-character changes. Rows are addressed by stable IDs.

3. **No notes field.** The attendance notes input is removed from the roster form;
   the form no longer sends `notes`, so stored notes stay untouched on edit and new
   rows get the empty default. Existing stored notes remain visible on the roster
   board.

The correction reason field is deliberately **not** removed: it is still rendered
only for archived raids, where a non-blank reason remains mandatory (ADR 032).

## Consequences

The roster form requires a resolvable character for every submission, so the stored
name comes from that character and class is read-only; only spec, role, and flags are
corrected there. The backend DTOs, schema, and validation are unchanged. Frontend
regression tests cover the locked class, the single `Ersatzbank` flag, and the
omitted `notes`.
