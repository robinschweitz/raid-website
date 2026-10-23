# ADR 095: Poll writes without a session use the common unauthenticated code

Status: Accepted.

## Context

FR-24.6 and OQ-10 (owner, 2026-10-22) required that a poll write without a session answer HTTP 401 `login_required`, "like every other protected endpoint". The code did not match that wording: every other protected endpoint answers 401 `unauthenticated` through `RequireAuth` and `RequireRole`. Implementing the poll-only code would have given poll writes a special case and a separate guard. Issue #92 was written against the same assumption.

## Decision

This ADR supersedes OQ-10 and the code part of FR-24.6.

- A poll write (create, vote, close) without a session answers HTTP 401 `unauthenticated` with the message `login required`, the same as every other protected endpoint.
- Poll writes use the existing `RequireRole` guard. No poll-specific guard exists.
- Role rules, the 403 answer for a role that is too low, and the "no `X-Actor-ID`" rule of SC-003 are unchanged.
