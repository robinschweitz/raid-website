# ADR 035: Raid name templates always derived from series when selected

## Decision
When creating a raid with a raid series selected (both manual creation and successor creation on archival), the raid name is always derived from the series naming template using the series' next counter at creation time. Any explicitly submitted name is ignored. When the template is empty, the series name is used. The series counter is incremented upon each derivation that uses a non-empty template (manual creation with a series, and successor creation). When no series is selected, an explicit name remains required.

## Status
Accepted (2026-10-06)

## Context
ADR 034 (Configurable Raid Name Templates) previously allowed manual raid creation with a series to accept an explicit name; in that case the submitted name was stored as given and the series counter was not consumed. Successor raids created during archival already always derived from the template (consuming the counter). The override behavior caused confusion: entering a name manually when a template-based series was selected broke the intended template numbering.

## Rationale
- Ensures consistent naming between manually created and automatically created (successor) raids for the same series.
- Prevents accidental breaking of sequential numbering by manual overrides.
- Simplifies the UX by making it clear the template governs naming when a series is selected.
- Aligns behavior with the owner's explicit request.

## Consequences
- API (manual creation): When series_id is provided, the backend always computes the name from raid_series.name_template (or falls back to name) using the scheduled start time as reference; the submitted name field is ignored. If the template is non-empty, next_raid_number is incremented and persisted in the same transaction.
- Archive/successors: No functional change (already derived from template). Behavior is now explicitly consistent.
- Frontend: The raid creation form no longer allows overriding the name when a series is selected. The name input is hidden in that case; the UI shows the derived name preview instead. Helper text is updated to reflect that the template always determines the name.
- Tests: Tests that assumed an explicit name overrides a series template were updated to match the new behavior. Counter is consumed only on derivation; with override removed, every manual creation with a series that uses a non-empty template consumes the next counter value.

## Implementation
- internal/raid/service.go (CreateRaid): Always derive the name from the selected series when SeriesID is set; ignore any submitted non-blank name. Reference time is the raid's scheduled start (UTC) or current UTC time if unset. Increment and save the series counter in-transaction when a non-empty template is used.
- internal/raidcompletion/service.go: Already always derives from template when non-empty (no change required).
- web/src/pages/Raids.tsx: Hide the name input when a series is selected; display the derived name preview. Updated explanatory text.

## References
- ADR 034: Configurable Raid Name Templates
