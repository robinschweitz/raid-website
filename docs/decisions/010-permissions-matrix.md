# ADR 010: Role-Based Permissions (Server-Side)

Status: Superseded in part by ADR 067

## Decision
Roles, from lowest to highest: `GUEST`, `MEMBER`, `RAIDASSISTENT`, `RAIDLEADER`, `ADMIN`.
The role is stored explicitly on the player row (`players.role`). Higher roles include all
powers of lower roles (`Role.AtLeast`). All mutations are checked server-side; the client
never decides authorization.

## Authentication
Local accounts with bcrypt-hashed passwords stored on the player (`username`, `password_hash`).
Login issues a random opaque session token stored (sha256-hashed) in `sessions` and sent as an
`HttpOnly`, `SameSite=Lax` cookie (`raid_session`, `Secure` when `COOKIE_SECURE=true` or
`ENV=production`). Sessions expire after 30 days and are validated on every request; an unknown
or expired cookie is treated as unauthenticated. Logout deletes the session.

Accounts are provisioned by an Admin via `PUT /api/v1/players/{id}/account` (username, password,
role). The first admin can be bootstrapped from `ADMIN_USERNAME`/`ADMIN_PASSWORD` at startup.

## Matrix
- Read (any authenticated role, i.e. `GUEST`+): all `GET` endpoints.
- Config (`ADMIN`): create player; set account/credentials; create/update characters; guild
  ranks; raid groups; list/import legacy data.
- Raid operations (`RAIDASSISTENT`+): create raid; complete/cancel/archive raid; attendance;
  loot; create/close polls.
- Self-service (`MEMBER`+): roll enhance points; create/cancel own item locks; vote on polls.

## Consequences
Sane RBAC. The legacy `X-Actor-ID` header is removed. Tests authenticate by logging in as seeded
accounts and exercise the role boundaries.
