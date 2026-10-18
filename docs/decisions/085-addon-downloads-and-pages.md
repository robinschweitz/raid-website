# ADR 085: Addon downloads, addon pages and dated invite lists

Status: Accepted. Amends ADR 045 and ADR 070 (invite export header).

## Context

Guild members installed RaidLedger and RaidWishlist from the `robinschweitz/wow-addons` GitHub
repository, which offers only the whole repository as a zip with the wrong folder layout, and they
had no place on the website explaining what the addons do. RaidLedger 2.2.0 can create a raid
event in the game calendar from the invite list, which needs the raid's date and start time.

The owner decided that the website serves the downloads itself, bundled into the site, with the
`wow-addons` repository as the source of the shipped versions.

## Decision

- **Bundle.** `internal/addons/bundle/<Name>/` holds the shipped files of RaidLedger and
  RaidWishlist: the `.toc` and exactly the files it lists. They are embedded with `go:embed`.
  At startup `addons.Load` parses each `.toc` (version, interface, file list) and builds one zip per
  addon whose single top-level folder is the addon name, so it unzips straight into
  `Interface\AddOns`. A missing listed file, a file the `.toc` does not list, or a `.toc` without
  version, interface or files stops startup, like the bundled catalog (ADR 027).
- **Updating.** `go run ./cmd/sync-addons <wow-addons checkout>` copies the `.toc` and its files
  from the checkout and removes files the new `.toc` no longer lists. It checks the whole source
  before writing. A new addon version reaches the guild by syncing, committing and deploying; the
  website version and the docs pages are changed in the same change.
- **API.** `GET /api/v1/addons` lists the bundled addons (name, version, file name, size, download
  URL); `GET /api/v1/addons/{name}/download` returns the zip (`application/zip`, attachment with
  `<Name>-<version>.zip`). Both are **public**: the addons are public on GitHub and contain no guild
  data. Unknown names answer 404 `addon_not_found`.
- **Pages.** `/addons` (overview, installation, a raid night with both addons),
  `/addons/raidledger` and `/addons/raidwishlist` (pitch, download, feature walkthrough with
  screenshots, website exchange formats, command reference, installation). German, readable
  without login like the guide (ADR 083), linked in the main navigation as "Addons" and from the
  guide, the invite export and the wishlist export. Mobile reaches them through the guide ("Hilfe").
- **Screenshots** in `web/public/addons/` are crops of real screenshots from live test runs on the
  local test realm (addon-forge `examples/.forge-runs/`), cropped to the addon windows or chat
  lines. They never show account names, passwords or the login screen. A unit test checks that
  every referenced image exists and nothing unused is shipped.
- **Dated invite list.** The invite export header gains the raid's date and start time:
  `RS2;INV;<raidID>;<raidName>;<size>;<YYYY-MM-DD>;<HH:MM>`, in realm time. Realm time is the
  raid series' timezone, the zone the raid is scheduled in. A raid without a series (no known
  timezone) or without a scheduled start keeps the five-field header; RaidLedger reads both.

## Consequences

- Shipping a new addon version needs a website deploy. In exchange the downloaded version always
  matches the docs page, and downloads do not depend on GitHub.
- The docs pages describe the addons in prose; when an addon changes its UI or commands, the
  pages and screenshots need updating in the same change as the bundle sync.
- Raids created without a series cannot get a calendar event from the invite list until they are
  assigned a series.
