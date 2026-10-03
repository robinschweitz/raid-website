# ADR 004: Enhance Points as Append-Only Ledger

## Decision
Store EP changes in `enhance_point_transactions` (amount +/-). Compute balances by SUM. Never UPDATE history; corrections = new entries.

## Consequences
Full auditability.
