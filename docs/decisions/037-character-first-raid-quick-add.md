# ADR 037 — Character-first attendance and inline raid onboarding

Status: Accepted

## Decision

The owner requested character-first attendance, class-dependent spec dropdowns, and
creation of first-time visitors without leaving the raid screen. The owner explicitly
approved RAIDASSISTENT and higher creating characters and, when necessary, their players
through this raid-scoped workflow. This extends ADR 010's creation permissions only for
the atomic raid quick-add operation; general player/character administration and account
credentials and roles remain Admin-only.

- Existing characters are selected directly from the authenticated character directory.
  Search supports character and player names; dropdown entries show character, class, and
  owner. Ownership is resolved from the selected stable ID rather than a prior player choice.
- A new character can belong to an explicitly selected existing player or a new player.
  The new player's editable name defaults to the character name. The existing GUEST default
  applies, with no username or password. An existing player name produces an explicit
  conflict requiring owner selection, never a silent merge.
- Class and main/second spec dropdowns use the server's existing ClassSpecs definitions.
  Raid attendance stores the chosen actual spec without changing the live character.
  Historical values remain selectable as saved values; merely opening or saving unrelated
  fields does not normalize history. Changing class clears incompatible draft specs.
- Creating the optional player, character, attendance, audit entries, and any completed-raid
  EP adjustment is one transaction. Existing correction rules govern archived reasons and
  cancelled/inconsistent raid rejection. Failed writes leave no orphan identities.
- A required request key is scoped to raid and actor. Identical retries return the existing
  attendance; reusing the key with different content conflicts. Keys and payloads persist
  atomically with the result, so retries cannot duplicate identities, attendance, or EP.

## Implementation

`raidcorrection.AddNewCharacter` owns the operation and checks staff permissions in addition
to route middleware. Migration 013 stores retry receipts. The authenticated GET endpoints
`/api/v1/characters` and `/api/v1/character-options` supply the directory and spec definitions.
`POST /api/v1/raids/{id}/quick-add` accepts no account credentials or account-role fields.

The raid roster offers `Neuen Charakter anlegen` and a single
`Anlegen und zum Raid hinzufügen` action. Player profiles share the spec dropdown component.
