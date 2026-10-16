# ADR 074: Admin audit-log access

Status: Accepted

## Decision

Admins can read the append-only audit log through `GET /api/v1/audit-log` and the
administration page "Audit-Protokoll". Entries are ordered newest first and include the actor,
entity, action, reason, and the stored before/after snapshots. The actor name is included when
the actor account still exists; otherwise the log retains its immutable actor ID.

The endpoint is restricted to `ADMIN`. The view is read-only and does not change, hide, or delete
audit entries.

## Consequences

Administrators can investigate historical corrections without database access. Server-side role
enforcement keeps audit snapshots unavailable to all other roles and unauthenticated visitors.
