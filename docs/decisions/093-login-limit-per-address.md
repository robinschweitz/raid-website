# ADR 093: Per-address login limit counts failed attempts only

Status: Accepted. Amends ADR 046 (login throttling); refines OD-010 (owner, 2026-10-23).

## Context

OD-010 added a second login limit next to SC-008: one client address may make at most 30 login
attempts, for any usernames, in 15 minutes (SC-043). It left open whether successful logins count.
Several guild members can share one address (a household, a LAN party), and every successful login
from it would otherwise use up the shared budget.

## Decision

- Only failed login attempts count toward the per-address limit: 30 failures from one address
  within 15 minutes block further attempts from that address until older failures leave the window.
- The check runs in `auth.StartLogin` before the per-username check of SC-008 and before any
  password comparison, and uses the `login_attempts` table (state survives restarts, unlike an
  in-memory counter).
- A rejected attempt is not recorded, so the block ends 15 minutes after the oldest counted failure.

## Consequences

- Password spraying across usernames from one address stops after 30 guesses per 15 minutes.
- Players sharing an address cannot lock each other out by logging in successfully.
- Successful login rows are no longer needed by any limit; their deletion follows SC-038 (CT-029).
