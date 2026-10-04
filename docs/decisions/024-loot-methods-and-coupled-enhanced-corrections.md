# ADR 024: Record Loot Methods and Coupled Enhanced Corrections

## Decision
RAIDASSISTENT and higher use a raid loot workspace. They select a boss, dropped item,
and recipient character from the raid roster, then record MAIN, SECOND, ENHANCED, or BOE.

MAIN and ENHANCED count as main-spec loot; SECOND and player BOE do not. There is no guild
bank recipient in this workflow. ENHANCED requires a positive balance and atomically
debits min(balance, 50). Only the recorded winner pays. Locks remain free main-spec awards,
and token transactions are derived from the designated token items.

Corrections require actor, reason, and before/after snapshots. An unchanged enhanced
recipient/method preserves its original cost even when notes or item metadata change.
Changing away from ENHANCED refunds its original debit. Changing the enhanced recipient
refunds the old winner, then charges min(new winner's current balance, 50), rejecting zero
balance. Changing to ENHANCED calculates the new cost from the current balance.

Award, compensation, token consequences, and audit share one transaction. Request retries
must not cause duplicate enhanced costs. Existing ledger entries are never overwritten.
New enhanced awards therefore require a stable, nonblank `request_id`. Same-key unchanged
retries return the stored award; a changed original payload conflicts. Existing corrections
operate on a stable award ID and preserve historical costs rather than charging again.
Archived/cancelled raids retain their write restrictions. Historical imports must not
recreate old enhanced costs from an ambiguous label or fabricate missing associations.
