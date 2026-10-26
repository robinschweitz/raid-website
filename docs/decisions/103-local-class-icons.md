# ADR 103: Class icons served locally, item icons stay on wow.zamimg.com

Status: Accepted. Amends DQ-2 (Owner 2026-10-22).

## Context

DQ-2 said every image comes from the site itself and the CSP drops `wow.zamimg.com`. The
class emblems were still hotlinked, so every page view told a third party the visitor's
address. The loot catalog also shows item icons from the same host.

## Decision

Owner, 2026-10-26:

- The ten Blizzard class icons are downloaded once and committed under
  `web/public/class-icons/`. `ClassEmblem` uses them; no class image is loaded from another server.
- Item icons in the loot catalog stay hotlinked from `https://wow.zamimg.com`. The CSP
  `img-src` therefore stays `'self' data: https://wow.zamimg.com`, for item icons only.

## Consequences

DESIGN.md (DQ-2, External assets, Do's) and SECURITY.md SC-024 say so. Moving item icons
local later needs a new decision.
