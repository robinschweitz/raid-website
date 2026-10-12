# ADR 050: Admin Account Merge

Accepted.

## Decision

- An ADMIN may merge an active source player into a distinct target player with a required reason.
- The target retains its password, role, and MFA configuration. Source sessions and all source MFA material are revoked.
- Source characters and item locks move to the target. A merge is rejected if both accounts have active locks for the same item in the same raid.
- The source is archived by setting `active` to false; it is never hard-deleted.
- The source EP balance moves through paired append-only `account_merge` transactions: a debit on the source and matching credit on the target.
- Every transferred record, authentication revocation, EP transfer, and source archival is actor/reason audited atomically.
