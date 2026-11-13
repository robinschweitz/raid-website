# ADR 113 — EP pools and an EP award per raid series

- Status: accepted
- Date: 2026-11-13
- Decided by: owner (2026-11-08)
- Amends: ADR 004, ADR 012, ADR 016, ADR 023, ADR 024 (one shared balance, fixed +10)

## Context

The guild raids more than Icecrown Citadel (ADR catalog: FR-25.5). Points earned in one raid
should be spendable only where the guild wants them: some raid nights share points, others
keep their own, and some give no points at all. Until now every player had one EP balance and
every archived raid awarded +10.

## Decision

1. **EP pools.** An ADMIN creates and renames named EP pools. A pool is a separate balance:
   every EP transaction and every item lock belongs to exactly one pool, and a player's balance
   is the sum of their transactions in that pool.
2. **Series choose a pool and an award.** A raid series has an optional pool and an EP award
   (whole number, 0 to 100, default 10). Several series may share one pool. A series without
   a pool, or with award 0, gives no points.
3. **Raids copy them.** A raid gets the pool and award of its series when it is created from
   the series or when its series is set. Changing a series does not change existing raids. A
   raid that already has EP transactions or item locks keeps its pool and award: setting
   another series that would change them is rejected (`409 raid_ep_locked`).
4. **The rules stay the same inside a pool.** Archiving awards the raid's award instead of a
   fixed +10; corrections add or take that award; ENHANCED costs min(balance, 50) of the
   recipient's balance in the raid's pool; a lock needs 150 EP in the raid's pool. A raid
   without a pool rejects ENHANCED awards and locks (`409 raid_has_no_ep_pool`); with award 0
   it awards nothing but still allows spending.
5. **Migration.** One pool named "ICC" is created; every existing transaction, lock, series,
   and raid joins it, so all balances stay exactly as they were.
6. **Display.** `/punkte`, the player profile, and the WL2 wishlist export show one pool at a
   time, chosen by the viewer; the first pool (lowest ID) is the default. With one pool the
   pages look as before. The addon formats do not change.

## Consequences

- The ledger stays append-only; old transactions are not recalculated (FR-16.7).
- Spending rules (50 cap, 150 lock threshold) are not configurable per pool (owner choice).
- A pool cannot be deleted; renaming is audited.
