# ADR 020: Core Integrity Boundaries

## Decision
Enforce the existing security, domain-first, and append-only requirements at write boundaries.
Self-service EP rolls and lock operations bind the target player to the session actor,
without an administrator ownership bypass. EP balance checks and debits share a transaction.

Raid completion, cancellation, and archive decisions read lifecycle state inside their
repository-owned transactions. Archiving is idempotent and cannot overwrite concurrent
completion. Completed completion remains idempotent; cancelled raids and archived active
raids cannot be completed.

Character corrections load their before-state inside the transaction and atomically persist
the update, name history, and audit log. They require a valid admin actor and nonblank reason.

Production requires explicit nondefault bootstrap credentials. Database initialization,
migration, and bootstrap failures abort startup. Bootstrap creates player and credentials
atomically and leaves existing accounts unchanged. SQLite foreign keys are enforced on
every connection, including replacement connections.

Unknown/expired sessions remain unauthenticated. Operational database failures are errors,
not invalid credentials or anonymous sessions. The HTTP layer reports them explicitly.

Frontend entity state and responses are scoped to the loaded entity; synchronous submission
guards prevent accidental duplicate actions. These are convenience and reliability measures,
not replacements for server authorization or transactions.

## Consequences
The modular-monolith stack remains unchanged. No dependencies are added. Regression tests
exercise ownership, concurrent operations, rollback, correction undo/redo, and import replay.
