# ADR 048: Rising-Gods Cloudflare Egress

## Status

Accepted

## Decision

Rising-Gods requests may be routed through the guild-owned Cloudflare Worker when the production host is blocked by the upstream service. The Worker only accepts authenticated GET requests for its root path and forwards them to the fixed `db.rising-gods.de` origin. It is not a general-purpose proxy.

The application enables this route only when both `RISING_GODS_PROXY_URL` and `RISING_GODS_PROXY_TOKEN` are configured. The token is sent as a bearer credential and stored only in deployment secrets. Public profile links remain direct Rising-Gods links.

## Consequences

Cloudflare is an additional trusted network provider for the requested public character data. A direct upstream connection remains the default and can be restored by removing both configuration variables.
