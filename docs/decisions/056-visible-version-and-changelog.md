# ADR 056: Visible version and changelog

Status: Accepted

## Decision

The application displays its version in the navigation and maintains its changelog in
`web/src/release.ts`. Every user-facing change increments the patch component of the visible
version and adds a concise German changelog entry in that same change.

## Consequences

The displayed version provides a simple release marker for guild members. Changes that affect only
internal implementation and have no user-visible behavior do not require a version increment.
