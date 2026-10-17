# ADR 077: Audit everyday changes, not only corrections

Status: Accepted. Amends ADR 006 and ADR 075.

## Context

The audit log recorded corrections of history (loot, attendance on completed raids, character
renames) and a few special actions. Ordinary writes left no trace: adding a character to an active
raid's roster, creating or cancelling a raid, wishlist changes, poll votes, item locks, guild
configuration, new players and account changes. Admins could not answer "who did this?".

## Decision

The owner asked for the audit log to also show ordinary changes, explicitly including roster
additions and wishlist changes. Every website write by a player or admin now writes one audit entry
with the actor, the entity and before/after snapshots:

| Entity (`entity_type`) | Actions |
| --- | --- |
| `raid` | `create`, `update` (name, schedule, instance, size, series), `cancel`, `archive`, `create_successor` |
| `attendance` | additions to planned/open raids are logged like corrections (`correction` with empty before) |
| `raid_series` | `create`, `update` |
| `wishlist_item` | `create`, `update` (priority, notes, roll choice), `delete` |
| `poll` | `create`, `close`, `vote` (poll title and the chosen options; previous choices as before) |
| `item_lock` | `create`, `cancel` |
| `guild_rank`, `raid_group` | `create`, `update` |
| `player` | `create`, `set_account`, `password_change`, `totp_enable`, `totp_disable`, `security_key_add`, `security_key_remove` |
| `registration_request` | `create`, `create_code`, `reject`, `redeem` |
| `registration_code` | `revoke`, `mark_sent` |

Rules:

- The entry is written in the same transaction as the change wherever the change itself is
  transactional, so a change never exists without its entry. Account-security changes, which were
  not transactional before, record their entry right after the change.
- Snapshots never contain secrets: no password hashes, passwords, TOTP secrets, recovery codes,
  registration codes or raw Rising-Gods profile JSON. Account changes record only whether the
  password changed.
- Sign-ins, sign-outs and MFA verification are not logged; they are session events, not changes.
- An entry with no signed-in player (for example a registration request from a visitor) has no
  actor and shows as "System".
- The Admin audit page translates the new entity types, actions, statuses and timestamps into German
  and links each entry to the affected page where one exists.

## Consequences

The audit log grows with everyday use. It stays append-only and Admin-only (ADR 074). Existing
tests that asserted "no audit entry for active-raid additions" now expect the addition entry.
