# ADR 066: Remove Google Sheets import and synchronization

Status: Accepted by owner, 2026-10-15.

## Decision

- Google Sheets is no longer a runtime integration. The application has no
  legacy import, automatic sync, website-to-Sheet delivery, or Sheet-related
  administration UI.
- Current domain data remains: players, characters, raids, attendance, loot,
  tokens, Enhance Point transactions, and their ordinary audit history.
- Import/sync provenance, mappings, previews, delivery queues, states, events,
  and their dedicated audit entries are permanently deleted by migration 066.
- A future legacy import is an offline, one-time Python operation, not a web
  feature or a recurring synchronization service.

## Consequences

ADRs 008, 019, 028, 029, 035, 057 through 065 are historical records only
where they govern Google Sheets import or synchronization. They no longer
describe active application behavior.
