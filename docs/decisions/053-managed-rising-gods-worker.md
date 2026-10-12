# ADR 053: Managed Rising-Gods Worker

## Status

Accepted

## Amends

ADR 048

## Decision

The Rising-Gods Cloudflare Worker source and Wrangler configuration are kept in
this repository. GitHub Actions deploys changes to the existing Worker using a
restricted `CLOUDFLARE_API_TOKEN` repository secret. The Worker authentication
secret remains exclusively in Cloudflare.

The Worker returns a request ID and upstream HTTP status in response headers.
The application retains those diagnostics in its internal unavailable error and
logs them for failed retry-link requests. User-facing errors remain generic.

## Consequences

Worker changes are reviewable, reproducible, and independently deployable.
Proxy or Rising-Gods failures can be correlated through the server journal and
Cloudflare Worker logs without exposing the proxy authentication secret.
