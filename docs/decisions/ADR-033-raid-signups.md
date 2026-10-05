# ADR 033 — Future raid signups and roster acceptance

Status: Accepted

## Decision

The owner requested future-raid signups and confirmed this workflow:

- RAIDASSISTENT and higher manage signup availability and acceptance. MEMBER and higher
  submit only their own characters. Reads follow the existing authenticated-read policy.
- Signup availability is independent of PLANNED/OPEN. Staff explicitly enables it for
  a raid with a future scheduled start; new raids and recurrence successors start disabled.
- Availability is evaluated using the server's UTC clock on every read and write:
  enabled, active/unarchived raid, and current time strictly before scheduled start.
  Staff can close early. The deadline follows the raid's current scheduled start.
- Players submit multiple character alternatives atomically, choosing any valid spec for
  each character's class. Saved main/second specs are not changed. One current choice per
  character per raid is retained; saving selected choices preserves the other choices.
- While open, players may edit or withdraw pending choices. Withdrawals retain the row.
  Accepted characters cannot be edited or withdrawn through self-service.
- Only one character per player is accepted through this workflow. Acceptance immediately
  creates the roster entry, including name/class/spec snapshots and the spec's tank/heal/DD
  role. Existing multiple roster entries for that player are reported as a conflict.
- Alternative choices remain available. Switching explicitly replaces the existing player
  roster slot via the audited correction service; the request identifies both the slot and
  its expected character so stale requests cannot silently replace a newer selection.
  An existing manually managed slot also requires explicit replacement confirmation.
- Acceptance reflects the actual roster rather than a second, independently mutable status.
  Ordinary staff roster corrections therefore remain authoritative. Repeated acceptance of
  the same character is idempotent and never resets subsequent staff spec corrections.
- Closing signups does not stop staff reviewing and accepting choices in active raids.
  Cancelled, completed, and archived raids cannot accept signups or change acceptance.
- Multi-character submissions return the persisted signup overview as a report. Signup
  writes and roster acceptance are actor-audited. Acceptance and its audit share a transaction.

## Implementation

`raidsignup` owns availability, ownership/spec validation, and acceptance orchestration.
Its repository owns SQL and binds `raidcorrection` to the same SQLite writer transaction.
No worker or scheduled job is necessary to enforce the deadline.

The German raid detail interface provides an Anmeldungen section, multi-character selection,
per-character specs, withdrawal, staff availability controls, and explicit alternative switches.
