# ADR 023: Track Outside Enhanced Rolls, Never Generate Them

## Decision
The owner restored the original product brief. The website tracks balances and history,
not dice rolls. Remove the self-service roll UI and endpoint. No player spend button,
slider, amount selector, or standalone EP-use action is exposed.

Outside the website, X = min(balance, 50) shifts the usual 1-100 roll to 1+X through
100+X. Examples: 30 EP means 31-130; 70 EP means using 50, rolling 51-150, and retaining 20.
Only the enhanced winner pays EP. Losing rolls do not consume points.

Raid staff records an enhanced winning loot award. Its ledger debit is part of that
transaction, never a generated roll. Existing ledger records remain unchanged history.
Final archive still awards +10 when no main-spec loot was received.

## Supersedes
The website-roll behavior and unshifted lower bound in ADR 011 are superseded. The automatic
50-point cap, inability to choose/split the amount, and append-only ledger remain.
