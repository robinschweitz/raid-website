# ADR 086: Shadowmourne questline list and raid assignments

Status: Accepted.

## Context

A first version of this feature (never merged) kept a strictly serial shard queue: one collector,
everyone else waiting. The owner rejected it: entries could not be removed, the shard target had
to be typed for every entry, progress editing was clunky, and members could not see who does which
questline step in a given raid. Several people can be in the questline at the same time, at
different steps.

## Decision

- The website keeps one shared Shadowmourne list. Each entry is a character with a **priority**
  (list position), a questline **stage**, a shard count and a shard target.
- Stages, in questline order: `PREQUEST`, `SOULS`, `UNHOLY` (Professor Putricide), `BLOOD`
  (Blood-Queen Lana'thel), `FROST` (Sindragosa), `SHARDS`, `FINAL`. Staff may set any stage,
  forwards or backwards; the website does not enforce transitions.
- Several entries can be active at once. A character can be on the list only once while active
  (partial unique index). Only `Krieger`, `Paladin` and `Todesritter` can be added, checked on the
  server.
- The shard target defaults to the Blizzlike **50** and stays editable per entry, for example when
  someone farmed shards in advance. Count and target are nonnegative, the target positive, and the
  count never exceeds the target.
- New entries go to the end or to a chosen position. Staff move an entry to any position; the
  others shift and priorities are renumbered 1..n.
- `Fertig` (complete) requires the shard target to be reached and moves the entry into the
  visible history with its UTC timestamp. `Entfernen` archives a wrong or abandoned entry
  (`removed_at`, ADR 007); it disappears from the list, stays in the audit log, and the character
  can be listed again.
- Per raid, the steps only one raid member can do (`UNHOLY`, `BLOOD`, `FROST`, `SHARDS`) show who
  does them. The **suggestion** is the active entry with the highest priority whose character is in
  that raid's roster and who is on that stage. Staff can store an explicit choice of any active
  entry in the roster, or reset to the suggestion. Choices cannot change for cancelled or archived
  raids.
- Every authenticated account reads the list (`GET /api/v1/shadowmourne`) and raid plan
  (`GET /api/v1/raids/{id}/shadowmourne`). `RAIDASSISTENT` and higher create, update, move,
  complete, remove and assign. Every write is audited with its actor (ADR 077).

## Consequences

Members see the order, everyone's step and, in the raid's roster tab, who does Seuche, Blut,
Frost and Splitter in that raid. Stages are maintained by hand; archiving a raid does not advance
anyone, because the website cannot know whether a step actually succeeded.
