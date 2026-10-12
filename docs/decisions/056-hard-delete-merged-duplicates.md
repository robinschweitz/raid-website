# ADR 056: Delete merged duplicate records

Status: Accepted by owner, 2026-10-12.

An ADMIN explicitly merging duplicate characters or player accounts permanently deletes the source record after all references have moved to the target in the same transaction. This supersedes the source-archival rules in ADR 050 and ADR 051.

Character merge aliases and audit entries retain the deleted source ID and name for traceability. Account merges move historical records, including Enhance Point transactions, to the target account before deleting the source. If an actor-scoped loot request ID conflicts after the move, its technical idempotency key is replaced without changing the award. For a duplicate poll vote or quick-add request key, the target's existing entry is retained.
