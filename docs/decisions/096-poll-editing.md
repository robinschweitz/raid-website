# ADR 096: Staff can edit a poll after creating it

Status: Accepted.

## Context

Polls could only be created and closed (FR-24.1). A typo in the title, a wrong option, or a
poll closed too early meant creating a new poll and losing the votes. The owner asked on
2026-10-24 that the raid lead can change a poll afterwards.

## Decision

- `PUT /api/v1/polls/{id}` (RAIDASSISTENT and higher) takes the full editable state: title,
  description, closing time, status `OPEN` or `CLOSED`, and for CHOICE polls the option list
  (`id` 0 adds an option, an existing `id` keeps it). Poll type and multiple selection stay as
  created.
- Setting the status to `OPEN` reopens a closed poll. An open poll cannot get a closing time in
  the past, because it would look open but reject every vote.
- An option keeps its identity when it is renamed or moved, so its votes stay and are shown with
  the new label (the label copies on stored votes are updated in the same transaction).
- Removing an option that has votes is rejected with `409 poll_option_has_votes`; the edit
  changes nothing. Options without votes can be removed.
- The edit writes one `poll` / `update` audit entry with the poll and its options before and
  after.

This amends FR-24.1; the rule is FR-24.7.
