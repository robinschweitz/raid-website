# ADR 049: On-demand Rising-Gods Lookups

## Status

Accepted

## Amends

ADR 044

## Decision

Rising-Gods list searches are made only in response to a user lookup. The bounded cache, stale-result handling, prefix reuse, and request coalescing remain in place. There is no startup or periodic whole-roster cache warming.

Upstream connection failures and non-success responses return the typed `ErrExternalUnavailable` error. The HTTP API maps it to HTTP 502 with the stable `rising_gods_unavailable` code.

## Consequences

The first uncached lookup can take several seconds, but the application does not issue Rising-Gods list searches without user demand. Cloudflare egress from ADR 048 remains unchanged.
