# ADR 052: Direct Profile Resolution for Retry Link

## Status

Accepted

## Amends

ADR 044 and ADR 049

## Decision

When an ADMIN retries linking an existing local character, the server resolves
the deterministic Rising-Gods profile slug `eu.rising-gods.<character-name>`
directly instead of running a broad character list search. The loaded profile
must still be an Alliance level-80 character and match the local character
name exactly before the ordinary audited link transaction persists it.

## Consequences

One retry no longer starts a potentially expensive Rising-Gods list search.
Failures remain explicit and do not fall back to a broad search, preventing
unexpected additional upstream traffic.
