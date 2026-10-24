# ADR 101: The website can be installed as an optional PWA

Status: Accepted.

## Context

The owner wants members to be able to open the raid tool from their phone's home screen
without an app store (issue #159, PR #160, owner 2026-10-24). Nothing may change for
anyone who does not install it, and no private data may be stored on the device.

## Decision

- The SPA ships a web app manifest (`/manifest.webmanifest`, `display: standalone`, German
  name, navy theme). Its icons are PNGs (192, 512, and 512 maskable) rendered from the guild
  crest, plus `apple-touch-icon.png` (180) for iOS. All are served from the site itself
  (DESIGN.md "External assets"). Go registers the `application/manifest+json` type, because
  the host MIME table differs per system.
- A service worker (`/sw.js`, scope `/`) is registered so that browsers which need one can
  offer installation. It has no `fetch` handler and uses no Cache Storage, so every request
  goes to the server exactly as without it. It handles no push messages. Web Push needs its
  own decision, a subscription store, and a security review.
- A thin install hint under the top bar, in the `info` colour, offers the browser's own
  install prompt where one exists (Chromium on Android). On iPhone and iPad it shows the
  manual Safari steps instead. It does not appear when nothing can be offered or the site
  already runs installed, and it can be hidden for the browser session. It is shown to
  visitors and signed-in users alike.
- The existing CSP (`default-src 'self'`, `script-src 'self'`) already covers the manifest
  and the worker; no header changes.

## Consequences

- An installed app is the same website in its own window. Logins, cookies, and every
  permission check work as in the browser.
- Offline use is not supported; without a network the installed app shows the browser's
  error page.
- A later change that adds caching or push must amend this ADR and SC-052.
