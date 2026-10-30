# ADR 107: Unclaimed loot in the loot form

Status: Accepted. Amends ADR 089 (manual entry of loot that went to nobody).

## Context

Staff entered awards and items that went to nobody in two separate forms. The second one took
a free-text item, so the same drop could be recorded under different names, and it left the
boss's kill unrecorded although the boss had clearly died.

## Decision

- **One form (owner, 2026-10-30).** "Vergabemethode" also offers Entzaubern, Gildenbank and
  Wollte niemand. These hide the recipient and store through `POST .../unclaimed-loot`.
- **Catalog pair required.** A manual entry needs a boss and an item that are a catalog pair of
  the raid's instance and size, checked on the server like an award (FR-14.2). Free text and
  "no boss" are gone for manual entries. Raids without an instance cannot take manual entries
  until staff set one. Existing entries stay as they are.
- **Kill in the same transaction.** Like an award, a manual entry creates a missing kill
  (FR-29.7, FR-29.12, FR-29.13). `raidcorrection.RecordUnclaimed` coordinates it, which also
  applies the raid-state rules of kill writes: an archived raid that was not completed takes no
  manual entry.
- **The entry is no kill evidence.** It stores no difficulty, so it does not block removing a
  kill or changing its difficulty (FR-29.9, FR-29.14).
- The addon import is unchanged (FR-20.10).
