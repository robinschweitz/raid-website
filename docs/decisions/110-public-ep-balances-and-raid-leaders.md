# ADR 110: Public EP balances and raid leaders

Status: Accepted. Amends ADR 067 (public reads) and the owner's answer to OQ-6 (no EP values in
public statistics), and ADR 109 (raid leaders shown only to signed-in users).

## Context

Visitors without a session could see raids, rosters, and statistics, but not who leads a raid
beyond the first leader, and not the Enhance Points. Guild members asked to check the EP
standing without logging in.

## Decision (owner, 2026-11-03)

- **Raid leaders are public.** `GET /api/v1/raids/{id}/leaders` answers without a session, so
  the overview and the raid page show all raid leaders to everyone. It returns player IDs only;
  names come from the public player list.
- **EP balances are public, the history is not.** A new public page `/punkte` ("EP-Stand")
  lists every active player with the current balance, sorted by balance; it shows no
  roll range. It reads `GET /api/v1/points`, which returns only player ID, player name, and
  balance. Single EP transactions (date, reason, raid) stay behind the login (FR-16.4).
- The public statistics keep their rule of no EP values (FR-27.2); the balance list is a page
  of its own, so the statistics contract does not change.
- The privacy page names the public balance list.
