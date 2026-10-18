# ADR 081: Member-friendly signup, absence and wishlist export

Status: Accepted.

## Context

Members struggled with the two things they do every week. Signing up or saying "I can't come"
required opening a raid and finding the "Anmeldungen" tab (raids opened on "Aufstellung"). The
wishlist and its addon export lived only at the bottom of each character page, reached through
"Spieler"; the export was a button below the table, and people who had added items could not find it.
Signing in from the "Anmelden" link also left members on "Seite nicht gefunden".

## Decision

Frontend only; no API, permission or data changes.

- **"Meine nächsten Raids" on the overview** for MEMBER and up: the next four raids that have not
  started, each with the member's status (Noch nicht angemeldet, Angemeldet, Bestätigt, Abgesagt,
  Anmeldung geschlossen) and direct actions:
  - "Mit <Main> (<Spec>) anmelden" signs up the main character (else the first active one) with its
    main spec, or the class's first valid spec. Other characters or specs go through the raid page.
  - "Kann nicht kommen" with an optional reason, and "Doch dabei" to take it back. These use the
    existing absence endpoints, so absence rules (until raid start, signup kept) are unchanged.
- **Raid pages open on "Anmeldungen" for members** on planned/open raids; staff keep "Aufstellung".
  `/raids/{id}/anmeldungen` links straight to the tab. The tab shows "Dein Status" and lists the signup
  form before the "Kannst du nicht kommen?" section.
- **"Meine Wunschliste"** (`/wishlist`) in the navigation (desktop and mobile) for MEMBER and up: the
  member's characters, defaulting to the main, with the full wishlist editor.
- **Addon export as a guided box** at the top of every wishlist: three numbered steps and one
  "Export kopieren" button that fetches the current export (with today's EP) and copies it. If the
  browser blocks the clipboard, the text is shown to copy by hand. An empty wishlist points to the
  loot catalog.
- **Loot catalog banner** for members: explains that clicking a character on an item adds it,
  confirms each add/remove (errors are no longer swallowed) and links to "Meine Wunschliste".
- **Login redirect:** `/login` sends a signed-in user to the overview.

## Consequences

The overview loads one signup overview per listed raid (at most four requests). Covered by unit tests
for the status logic and end-to-end tests of the member journey.
