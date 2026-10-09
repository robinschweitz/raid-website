# ADR 041: Armor Tokens Derived From Loot

## Decision
Armor tokens are not a separate ledger. They are derived from loot awards whose item name matches one of the three token items: `Weiheabzeichen des Beschützers`, `Weiheabzeichen des Bezwingers`, or `Weiheabzeichen des Eroberers`.

Player profiles expose token information by filtering the player's loot history. The backend may return a derived `token_award` flag on loot responses, but it must not persist token balances or token transactions.

This supersedes ADR 005 and the transaction-creation part of ADR 014.

## Consequences
The `token_transactions` table and `internal/tokens` ledger package are removed. Corrections and imports update/audit loot awards only; token information follows from the current audited loot state.

The legacy sheet's `Tokens gesammelt` total remains reconciliation-only and is never written over derived loot data.
