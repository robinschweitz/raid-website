# ADR 048: On-demand Rising-Gods lookups

Status: Accepted

Amends: ADR 044

## Context

The cache warmer added with ADR 044 searched Rising-Gods once for every active local character at
server startup and repeated that work every six hours. The traffic scaled with the local roster rather
than actual lookup demand and could create a sustained upstream request burst after deployment.
Rising-Gods may reject those requests with HTTP 403, leaving registration and every other interactive
character lookup unavailable.

## Decision

Rising-Gods list searches are performed only in response to a user lookup. Successful results retain
ADR 044's bounded cache, stale-result handling, prefix reuse, and request coalescing. There is no
startup or periodic whole-roster cache warming.

Upstream connection failures and non-success HTTP responses are exposed as the typed
`ErrExternalUnavailable` error. The HTTP API maps that error to status 502 with the stable code
`rising_gods_unavailable`, so clients no longer present an upstream outage as a generic registration
failure.

## Consequences

The first lookup for an uncached query can still take several seconds, but the application sends no
Rising-Gods list-search traffic without a corresponding user request. A temporary upstream rejection
can recover without the background warmer continually issuing more requests.
