# ADR 102: Opt-in Web Push for new polls, new raids, and own roster places

Status: Accepted. Amends ADR 101 (the service worker now handles push messages).

## Context

ADR 101 made the website installable and kept push out of scope: nothing could send a
message, and SC-052 forbade a push handler. The owner wants members to learn about new
polls without opening the site, and decided on 2026-10-24 (questions in the session that
built this change):

1. **Events:** a new poll, a new raid, and the player's own character entering a raid roster.
2. **Implementation:** RFC 8291 message encryption and RFC 8292 VAPID with the Go standard
   library only; no new Go dependency (DR-11).
3. **Key:** the VAPID private key lives in `/etc/raid-website.env` (SC-033), which the daily
   encrypted off-server copy already contains (SC-034). Without a key, push is off and the
   UI offers nothing; production still starts.

## Decision

- **Opt-in per device.** A MEMBER or higher turns notifications on in the own account tab.
  The browser's permission prompt appears only after that click. There is one switch for
  all three events; per-event settings are not built.
- **Package `internal/push`** owns `push_subscriptions` (player, endpoint, `p256dh`, `auth`,
  created_at; endpoint unique). `POST /api/v1/push/subscriptions` (MEMBER and higher) stores
  the own subscription, `DELETE /api/v1/push/subscriptions` (any signed-in player) removes it,
  `GET /api/v1/push/config` (MEMBER and higher) returns whether push is on and the public
  key. Writes are audited with the push-service host only (SC-031, SC-053).
- **Endpoints are restricted** to the push services of Chrome/Edge (FCM), Firefox, Safari and
  Windows, over HTTPS on port 443, and the sender follows no redirects, so a client cannot
  make the server post to an address of its choice (SC-053).
- **Sending** happens in the request that made the change, after its transaction has
  committed and after its answer has been flushed: no background job, and an error is
  logged but never fails the write. At most four messages go out at once, all within 15 s.
  The author of a change gets no message about it. Recipients are active players with a
  login and at least the MEMBER role at sending time.
- **Neutral content.** A message carries only its kind (`poll`, `raid`, `roster`). The service
  worker shows a fixed German text for it and opens the matching page on click; titles,
  names, and times never leave the server in a push.
- **Clean-up.** Logout deletes the device's subscription first and unsubscribes the browser.
  A 404 or 410 from the push service deletes the row. An account merge deletes the
  source's rows with its other credentials (FR-5.4).
- **New raids** are detected by the raid ID rising during the write (manual create, series
  save, series planning, archive with successor). One write gives one message, also when it
  planned several raids. **Roster** messages go to players whose roster row is new after
  the write, in a planned or open, unarchived raid; corrections of finished raids stay quiet.

## Consequences

- Push is delivered by Google, Mozilla, Apple, or Microsoft. They see the endpoint and the
  timing of a message but not its content (it is encrypted end to end).
- iPhones and iPads get push only for the installed web app on iOS 16.4 or newer; the
  account tab says so.
- An expired session does not delete a subscription, so a device that was never logged out
  keeps getting the neutral texts until the player switches them off or the browser drops
  the subscription. Accepted because the texts contain no data.
- Rotating the VAPID key invalidates every subscription; players must switch notifications
  on again.
- Adding events, content, or caching to the worker needs a new decision.
