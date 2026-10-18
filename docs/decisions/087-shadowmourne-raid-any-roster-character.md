# ADR 087: Shadowmourne raid steps accept roster characters without a list entry

Status: Accepted. Amends ADR 086.

## Context

ADR 086 let raid staff choose only active Shadowmourne list entries for the single-person raid
steps (`UNHOLY`, `BLOOD`, `FROST`, `SHARDS`), and offered every list entry in the roster for every
step. The owner found that wrong in two ways: someone already collecting shards was offered for
the blood infusion, and characters in the raid who are not on the list could not be chosen at all.

## Decision

- A raid choice stores a **roster character** (`character_id`), not a list entry. Migration 073
  converts existing choices to the character of their entry.
- For each step, staff may choose:
  - characters whose active list entry is **on that step**, in list priority order, and
  - `Krieger`, `Paladin` and `Todesritter` from the raid roster that are **not on the active list**.
  Characters on the list at another step, other classes, and characters outside the roster are
  rejected by the server.
- The suggestion is unchanged: the highest-priority list entry in the roster on that step.
- A stored choice counts only while the character is in the raid roster; otherwise the step falls
  back to the suggestion.
- Members see the card once a step has a suggestion or a choice. Everything else in ADR 086 stays.
