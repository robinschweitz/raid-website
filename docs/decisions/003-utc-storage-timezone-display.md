# ADR 003: UTC Storage, Client TZ Display

## Decision
Persist all timestamps in UTC. Format for display using user's local timezone in frontend.

## Consequences
Correctness across timezones; explicit handling on import.
