# ADR 092: MFA reminder banner and account-security page

Status: Accepted. Amends ADR 046 (MFA stays optional for every role except ADMIN; Admins must set it up, FR-3.10 and SC-040, owner 2026-10-23).

## Context

ADR 046 made MFA optional, and many players never set it up. The owner wants players to be
nudged towards a second factor and to understand why it matters, backed by real, published data
from reputable sources (Microsoft, Google and others) rather than vague warnings.

## Decision

- MFA stays optional. Nothing is blocked or enforced.
- Every signed-in player whose account has **neither** an enabled authenticator app (TOTP) **nor**
  a registered WebAuthn security key sees a thin reminder line under the top bar. Recovery codes
  alone do not count as a second factor.
- The reminder uses the existing `GET /api/v1/auth/mfa`; the server stays the source of truth.
  If the lookup fails, no reminder is shown.
- The reminder can be hidden for the current browser session (`sessionStorage`); it returns on the
  next visit until a second factor exists. Setting up or removing a factor on the profile page
  updates the reminder without a reload.
- Clicking the reminder opens `/kontosicherheit`, a German explainer page. It shows the player's
  own status, cited statistics, what an attacker could do with an account on this site, the two
  methods this site supports, setup steps and a numbered source list. Every figure links to its
  source; no figure may appear without one (ADR 026).
- `/kontosicherheit` is readable without login, like the guide (ADR 083).
- `/players/{id}/konto` opens the profile directly on the account tab.

## Sources used on the page

- Microsoft Security Blog, 20 Aug 2019: MFA blocks over 99.9 % of account compromise attacks; up to
  73 % of passwords are duplicates; over 300 million fraudulent sign-in attempts per day.
- Microsoft Research, Meyer et al. 2023: MFA reduces compromise risk by 99.22 %, 98.56 % with leaked
  credentials.
- Google Security Blog with NYU and UCSD, 17 May 2019: an SMS code blocked 100 % of automated bots
  and 96 % of bulk phishing; no security-key-only user fell to targeted phishing.
- Verizon DBIR 2025: credential abuse was the top initial access vector at 22 %.
- BSI recommendations and assessment of 2FA methods; CISA "More than a Password"; Have I Been Pwned.

## Consequences

- Updating a figure means updating the page, this ADR's source list and the frontend test that
  checks every cited source exists.
- If the owner later wants MFA enforced for some roles, that is a new decision.
