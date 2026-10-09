# ADR 039: Choice polls with optional multiple selection

Status: Accepted

## Decision

Polls support two answer modes:

1. `TEXT` polls keep the existing free-text answer field. They do not accept predefined
   options and cannot enable multiple selection.
2. `CHOICE` polls define an ordered list of answer options. Raid staff can configure
   whether one or multiple options may be selected by each voter.

Each player still has one vote record per poll. For choice polls, the selected options
are stored as ordered vote choices and the existing `choice` field remains a readable
summary for list views and older UI code.

## Consequences

The API exposes `poll_type`, `allow_multiple`, and `options` on polls. Vote requests may
send `choice` for free text and single-option compatibility, or `choices` for choice
polls. The server validates that choice-poll votes reference only configured options and
rejects duplicate or too many selections.
