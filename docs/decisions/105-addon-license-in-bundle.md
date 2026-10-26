# ADR 105: An addon's LICENSE ships in its bundle and download

Status: Accepted. Amends ADR 085 (Bundle, Updating).

## Context

RaidWishlist 2.3.0 is GPL-2.0-only, and its raid catalogue is derived from the GPL-2.0
AzerothCore world database. The GPL requires a copy of the license with every distributed
copy. ADR 085 limited the bundle to the `.toc` and exactly the files it lists, so the website
download, the copy most raiders install, would ship without the license text.

## Decision

Owner, 2026-10-26:

- An addon's bundle folder MAY also hold a `LICENSE` file from the wow-addons checkout. It is the
  only file outside the `.toc` list that the bundle accepts.
- `cmd/sync-addons` copies `LICENSE` when the source addon has one and removes it when the
  source has none, like any other file of the old version.
- The download zip contains `<Name>/LICENSE` whenever the bundle has it.
- Any other file the `.toc` does not list still stops startup.

## Consequences

REQUIREMENTS.md FR-21.2 says so. An addon without a license file ships as before. Raiders get
the license text inside the addon folder they unzip into `Interface\AddOns`; the game ignores it.
