# ADR 068: Raid workspace ergonomics

Status: Accepted

## Decision

The raid workspace keeps native form controls and does not add a frontend dependency for
selection widgets. Character and catalog inputs use their native suggestion and keyboard
behavior. A selected value is visible and may be explicitly cleared without leaving the
underlying attendance or loot references behind.

Roster filters are display-only. They never change roster records and keep an active in-place
editor visible so its draft is not lost. Raid mutations show a local success status after the
server accepted them. The roster and loot workspace use sticky section headings and compact
mobile controls while preserving the existing desktop-first ICC visual language.

## Consequences

The raid page remains dependency-free and server authorization is unchanged. Filters and status
messages are local UI state only; API payloads and the underlying raid history are unaffected.
