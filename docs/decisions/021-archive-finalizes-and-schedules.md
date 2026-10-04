# ADR 021: Archive Finalizes the Raid and Schedules Its Successor

## Decision
The owner clarified that archiving is final processing, not removal. The owner selected
one final action instead of separate completion and archive stages. `Archivieren` atomically
sets `COMPLETED`, `completed_at`, and `archived_at`, awards eligible EP, expires unused
locks, and schedules the next PLANNED raid. Cancelled raids cannot be finalized or earn EP.
The separate completion API/action is removed. Raid history remains visible by default.

The owner selected configurable schedules per raid series, with weekday(s), an interval
in weeks, local start time, and IANA timezone. Each series also configures the name and
leader used for newly created successors. Administrators manage configuration; raid
assistants and higher create raids, set their series/start, and finalize them. No guild
schedule, timezone, cadence, or leader is seeded.

Eligible weeks use the current raid's local Monday week as their anchor. A successor must
be strictly later than both the current scheduled start and finalization time. The owner
selected skipping elapsed occurrences, not creating overdue raids. Calendar arithmetic
preserves configured local wall time across timezone-offset changes. DST gaps and folds
produce explicit errors rather than selecting an unconfirmed shift/offset policy.

An eligible existing PLANNED/OPEN occurrence can be linked without overwriting its name,
leader, notes, roster, or timestamps. Cancelled, completed, archived, or differently linked
occurrences produce conflicts. Unique occurrence and predecessor constraints, combined
with serialized writes, prevent duplicate successors. Retrying returns the saved successor.

## Persisted History
Existing stored COMPLETED raids may already contain raid rewards or imported opening
balances. Archiving these records only marks them archived and returns any already linked
successor; it never regenerates rewards or starts a historical recurrence chain. No
migration awards points or creates future raids automatically.

Existing active raids need an explicit series and scheduled start before finalization.
Both can be configured through the raid PATCH endpoint. Missing prerequisites, incomplete
rosters, invalid local schedule times, or persistence failures leave all records unchanged.

## Supersedes
For raids, this supersedes ADR 007's removal/hiding interpretation and ADR 020's separate
completion/archive actions. Append-only ledgers, cancellation rules, audited corrections,
and deletion restrictions remain unchanged. Finalized roster and loot are read-only.
