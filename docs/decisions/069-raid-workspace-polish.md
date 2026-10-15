# ADR 069: Raid workspace polish

Status: Accepted. Amends ADR 068.

## Decision

Picker inputs for attendance characters and catalog items use the full form width, and the
selected value is shown in a full-width selection row with a clear action. Success messages stay
visible while the user scrolls, can be closed, and are cleared when the user switches the raid
tab; they do not use timers. While a roster filter is active, empty roles and the bench say
"Keine Treffer" instead of "Noch nicht besetzt", and "Filter zurücksetzen" sits with the match
count. On small screens the raid tabs form a two-column grid instead of a scrolling strip, and
the attendance role selector has a visible label.

Only loot boss headings stay sticky. Roster column headings are no longer sticky: the columns are
short and have different heights, so their headings floated over partly hidden members.

## Consequences

The raid page remains dependency-free and the visual language is unchanged. Messages and filters
are local UI state only; API payloads, authorization and raid history are unaffected.
