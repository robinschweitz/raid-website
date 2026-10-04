# ADR 017: Reject Competing Item Locks

## Decision
The owner confirmed that a raid may have only one matching active lock for an item.
Competing locks, including a second lock by the same player, are rejected. Players may
still hold locks for multiple different items, as in ADR 012.

When both records have nonempty item IDs, the trimmed IDs define identity. Otherwise,
the exact trimmed item name is used. A nonblank item name is required. Creation checks
ownership, raid lifecycle, the unchanged 150 EP threshold, and competition atomically.

An ordinary drop is awarded automatically to the active locker as main-spec loot, and
the lock becomes USED in the award transaction. Existing ambiguous locks fail explicitly;
the application never chooses a winner silently.

## Consequences
There is no new lock cost or limit on distinct items. Members, including administrators
using self-service, may create and cancel only their own locks. Completed, cancelled,
and archived raids accept no new locks.
