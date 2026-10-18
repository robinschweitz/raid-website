# ADR 083: In-app user guide ("Anleitung")

Status: Accepted.

## Context

The owner wanted a simple, picture-based guide for every role (visitor, Gast, Mitglied,
Raidassistent, Raidleitung, Admin) and expected it on the website itself, not in a separate
document. New people need it before they have an account, because the registration flow
(Code anfordern, Registrieren) is part of what it explains.

## Decision

Frontend only; no API, permission or data changes.

- **Page `/anleitung`** (`web/src/pages/Guide.tsx`), linked as "Anleitung" in the main navigation
  (mobile: "Hilfe") for everyone, and from the login card ("Neu hier? Anleitung mit Bildern lesen").
- **Readable without login.** The client gate lets anonymous visitors open `/` and `/anleitung`.
  The guide is static text and images; it fetches no data, so this exposes nothing the API
  would otherwise protect (authorization stays server-side, ADR 010).
- **Structure:** a "Wofür brauchst du Hilfe?" chooser (Neu hier, Mitglieder, Raidleitung, Admins)
  that highlights the reader's own area by role, a chapter list, then one card per chapter with
  numbered steps, worked examples from a fictional guild, and screenshots. All chapters are visible
  to every role; the guide hides nothing, it only points each reader to their part.
- **Screenshots** are WebP files in `web/public/anleitung/`, served as static assets and loaded
  lazily. They are generated, not hand-made: `DOCS_TOUR=1 npx playwright test
  e2e/zz-docs-tour.spec.ts` (after `npm run build`) seeds the example guild in a fresh e2e database,
  captures each screen, and converts it to WebP inside Chromium, so no image dependency is added.
  The full-size PNGs go to the gitignored `web/docs-screenshots/`.
- A unit test checks that every screenshot the guide references exists and that no unreferenced
  image is shipped.

## Consequences

- UI changes can make the guide outdated. Whoever changes a described screen updates the text in
  `Guide.tsx` and reruns the tour to refresh the images in the same change.
- The images add about 1.6 MB to the repository and the built site; they are only downloaded
  when the guide is opened.
