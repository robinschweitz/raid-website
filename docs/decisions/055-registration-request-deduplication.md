# ADR 055: Registration request deduplication

Status: Accepted

## Decision

Registration requests are unique per Rising-Gods profile while an earlier request is `PENDING` or
`APPROVED`, and after it has been `CONSUMED`. A rejected request may be submitted again.

The application checks this before loading the external profile for a clear client error. A SQLite
insert trigger enforces the same rule atomically, including concurrent requests. Existing duplicate
records are retained as history and are not changed by the migration.

## Consequences

The public registration and character-claim endpoints return HTTP 409 with
`registration_request_exists` when a blocked request exists. Administrators can reject an unwanted
request, after which a new request for that profile is possible.
