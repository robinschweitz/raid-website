# ADR 051 — Merge mistakenly duplicated characters

Status: Accepted by owner, 2026-10-12.

A character misspelled during a legacy import is not a historical in-game rename. An ADMIN may explicitly merge the duplicate into the correct existing character with a required reason, after merging player accounts if they differ.

Move historical character references and their incorrect stored name snapshots to the correct character/name in one transaction. The corresponding attendance, loot, and signup player references move to the target owner too, so every rewritten character reference still belongs to its stored player. Preserve source character as inactive and preserve old identity and actor/reason in audit log. Never rewrite append-only EP transactions or recreate EP awards. Conflicting records for the same raid or wishlist item reject the entire change; do not guess which row to discard.

Persist an explicit source-name alias for future legacy imports so the typo does not re-create a character. Do not infer aliases via Unicode normalization or automatic character-name matching.

Historical changes only occur when the administrator initiates the merge; no blanket migration of all names.
