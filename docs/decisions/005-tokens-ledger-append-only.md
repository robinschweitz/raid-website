# ADR 005: Armor Tokens as Append-Only Ledger

Superseded by ADR 041.

## Decision
Store token changes in `token_transactions`, append-only. Track via the three specific token loot items (Beschützer/Bezwinger/Eroberer). Sheet total only for reconciliation in dry-run.

## Consequences
Prevents silent overwrite from sheet totals.
