# ADR 054: Rising-Gods Proxy Client Identification

## Status

Accepted

## Amends

ADR 048 and ADR 053

## Decision

The Rising-Gods client sends the fixed browser-compatible User-Agent
`Mozilla/5.0 (compatible; Bratwurst-des-Schicksals/1.0)` for direct and
Cloudflare-proxied requests.

## Consequences

Cloudflare can distinguish the guild service from anonymous default HTTP
clients and no longer rejects its requests before the authenticated Worker.
The User-Agent contains no player, account, or request-specific data.
