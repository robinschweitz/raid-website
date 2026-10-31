# ADR 106: Admin recovery for undelivered registration codes

Status: Accepted.

## Context

The owner reported on 2026-10-23 that an Admin created registration codes but did not send them. A different Admin needs a way to finish onboarding.

## Decision

Registration codes remain hash-only at rest, as required by ADR 040. It is not possible to retrieve the original plaintext code from the database. To resolve missing deliveries without weakening code storage:

- The Admin registration UI explains why a previously created code cannot be shown again.
- Admins can inspect existing code metadata and see whether it was used, revoked or marked as sent and when it expires.
- An Admin may issue a fresh code through the existing authorized API and copy the returned plaintext once, and revoke obsolete unused codes.
- No plaintext code is persisted and no new secret-disclosure API is introduced.

This amends ADR 040 only in the administrative recovery workflow.
