# ADR 067: Public dashboard reads

Status: Accepted

Supersedes the read rule in ADR 010.

## Decision

Visitors may view the dashboard without a session. The only unauthenticated API reads are
`GET /api/v1/raids`, `GET /api/v1/players`, `GET /api/v1/polls`, and
`GET /api/v1/raids/{id}/attendance`.

The public player list exposes only display-safe data: ID, name, guild rank, and active state.
All other reads, including player-specific points, characters, and poll votes, remain
authenticated. Every mutation retains its existing server-side authorization check.

## Consequences

The dashboard hides personal profile information for visitors and does not request protected
personal data. The public read boundary is covered by HTTP and frontend regression tests.
