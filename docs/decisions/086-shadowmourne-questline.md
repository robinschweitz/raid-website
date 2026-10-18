# ADR 084: Visible Shadowmourne shard priority

Status: Accepted.

## Decision

The guild completes Shadowmourne serially: one character collects the required shards and
finishes the weapon before the next character may collect. The website therefore keeps a
single shared priority queue.

- Every authenticated account can read `GET /api/v1/shadowmourne-queue` and see the active
  collector, waiting order, shard progress, and completed history.
- `RAIDASSISTENT` and higher create entries, update the active collector's collected-shard
  count and target, reorder waiting entries, and complete the active entry.
- The first entry becomes `COLLECTING`; later entries are `QUEUED`. At most one entry can be
  collecting, enforced by a partial unique database index.
- Progress is a manually maintained nonnegative count up to a positive, explicitly stored
  target. The target is not hard-coded. An entry can only be completed once it reaches that
  target. Completion records its UTC timestamp and atomically promotes the first waiting
  entry, if one exists.
- Completed entries remain visible as history. Every create, progress update, reorder, and
  completion writes an audit record with before/after snapshots.

## Consequences

The assigned shard order is visible without relying on chat messages or an external list.
The guild can use a target appropriate to its server or rules without changing the software.
