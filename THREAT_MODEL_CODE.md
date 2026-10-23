# Code Threat Model — raid-website

This file owns threats found in the **implementation**: the code on `main`
(`376185e`, v1.0.27), its dependencies, CI workflows, and repository settings, reviewed on
2026-10-23. It complements `THREAT_MODEL.md`, which models the specification only. Code threats
have stable IDs `CT-nnn` that are never reused. Each links the `SC-*` rule it breaks; where no rule
existed, a rule was added to `SECURITY.md` and a spec threat to `THREAT_MODEL.md`.

## Method

| Technique | What was done |
| --- | --- |
| Software composition analysis | `govulncheck v1.1.4` with the toolchain `go.mod` names (`go1.26.0`); `npm audit` for runtime and dev dependencies |
| Pattern search | SQL built from strings, raw SQL, error text sent to clients, URL and HTML sinks in the client, unchecked indexing in parsers, secrets in JSON |
| Manual secure code review | Every state-changing handler that takes an ID: ownership and role checks in the domain service; sessions; MFA and WebAuthn ceremonies; registration and claim codes; account administration; CI workflows |
| STRIDE per code component | Category column in the register |
| OWASP Top 10:2021 checklist | [Top 10 checklist](#owasp-top-10-checklist) |
| Spec conformance | Each `SC-*` rule's **Evidence** checked against the code; results in the register |

Status values: `CONFIRMED` (seen in the code), `FIX PENDING` (a fix is open), `ACCEPTED` (owner
decision or design), `DECISION` (needs an owner decision). Each threat has a Shostack response.
No numeric risk scores; the **Impact** line says in words what an attacker gains.

## Register

### Dependencies and toolchain

**CT-001 — Known vulnerabilities in the Go standard library**
- **Category:** Denial of service, Information disclosure, Spoofing. **Component:** whole binary.
- **Evidence:** `govulncheck` reports 29 vulnerabilities whose vulnerable functions the code
  actually reaches with `go1.26.0`, among them HTTP/2 server memory exhaustion and crash
  (GO-2026-6603, GO-2026-6617), unbounded Range headers (GO-2026-6609), request desynchronisation
  (GO-2026-6613), and several `crypto/x509` flaws reached through WebAuthn (GO-2026-4866,
  GO-2026-6809). All are fixed in `go1.26.10`. `go.mod` says `go 1.26.0` and has no `toolchain`
  line, so a host whose Go is older than `1.26.10` builds a vulnerable binary; the production
  build scripts are not in the repository (CT-017), so the host's Go version is unknown. CI uses
  the newest `1.26.x`. 19 further findings in modules are not reachable.
- **Impact:** a remote visitor can crash or slow the server; certificate checks in WebAuthn are
  weaker than intended.
- **Status:** CONFIRMED. **Response:** Mitigate. **Rules:** SC-045, SC-039.

**CT-002 — Frontend dependencies**
- **Evidence:** `npm audit` reports 0 vulnerabilities (runtime and dev).
- **Status:** none found.

### Authentication and accounts

**CT-003 — Username probing through registration**
- **Category:** Information disclosure. **Component:** `registration.RedeemNewAccount`
  (`internal/registration/service.go:554`).
- **Evidence:** the "username already taken" check runs *before* the registration code is
  validated, and the endpoint is public and not rate-limited. Anyone can send any username with a
  made-up code and read `409 username_taken`. A race between that check and the insert surfaces
  as a database error instead.
- **Impact:** a list of valid login names for password guessing (helps CT-026).
- **Status:** CONFIRMED. **Response:** Mitigate. **Rules:** SC-046 (new), SC-028.

**CT-004 — Username probing through login timing**
- **Evidence:** `auth.StartLogin` returns before any bcrypt comparison for unknown usernames.
- **Status:** CONFIRMED. **Response:** Mitigate. **Rules:** SC-009.

**CT-005 — A new login can attach to another player by character name**
- **Category:** Spoofing, Elevation of privilege. **Component:** `RedeemNewAccount`.
- **Evidence:** when no character is linked to the requested Rising-Gods profile, the code falls
  back to `FindByName(req.CharacterName)` and gives the new login to the player who owns that
  character — if that player has no login yet. Character names are not unique and can be reused
  after renames (FR-6.3). ADR 084 describes this fallback on purpose for players imported before
  logins existed.
- **Impact:** someone whose new character carries the name of an old imported character gets that
  player's history and EP. The Admin issues the code and can see the linked player afterwards
  (FR-4.9).
- **Status:** DECISION — OD-016. **Rules:** FR-4.5.

**CT-006 — The last Admin can be demoted**
- **Category:** Denial of service (administration). **Component:** `auth.SetCredentials`.
- **Evidence:** any Admin can set any player's role, including their own and the last Admin's;
  there is no guard. `BootstrapAdmin` never changes an existing account, so a restart does not
  restore access.
- **Impact:** nobody can administer the site without editing the database by hand.
- **Status:** CONFIRMED. **Response:** Mitigate. **Rules:** SC-047 (new), FR-5.9 (new).

**CT-007 — Internal error text returned to clients**
- **Category:** Information disclosure. **Components:** `internal/httpapi/v1/auth.go:428`
  (`setAccount`), and handlers that pass `err.Error()` of wrapped domain errors
  (`character_merge.go`, `characters.go`, `players.go`, `raids.go`, `raidseries.go`).
- **Evidence:** `setAccount` answers every unexpected error, database failures included, with
  `422` and the raw error chain (for example a constraint name or `bcrypt: password length exceeds
  72 bytes`). The others send sentinel texts that may carry wrapping context.
- **Impact:** small (Admin-only or staff-only), but it breaks SC-023 and misreports server errors
  as client errors.
- **Status:** CONFIRMED. **Response:** Mitigate. **Rules:** SC-023, SC-005.

**CT-008 — Cloned security keys are not detected**
- **Category:** Spoofing. **Component:** `auth.FinishWebAuthnMFA`, `updateWebAuthnCredential`.
- **Evidence:** the WebAuthn library reports a clone warning when the authenticator's signature
  counter goes backwards; the code stores the new counter but never checks the warning.
- **Impact:** a copied authenticator keeps working unnoticed (rare for hardware keys).
- **Status:** CONFIRMED. **Response:** Mitigate. **Rules:** SC-048 (new).

**CT-009 — MFA codes can be guessed without limit** — `VerifyMFAChallenge` has no failure counter.
- **Status:** CONFIRMED. **Response:** Mitigate. **Rules:** SC-011.

**CT-010 — TOTP codes can be replayed** — no last-used time step is stored.
- **Status:** CONFIRMED. **Response:** Mitigate. **Rules:** SC-012.

**CT-011 — TOTP secrets stored in plaintext** — `mfa_settings.totp_secret`.
- **Status:** CONFIRMED. **Response:** Mitigate. **Rules:** SC-014.

**CT-012 — Sessions outlive password and role changes; no idle timeout** — only account merge ends
sessions; validation checks only the 30-day expiry.
- **Status:** CONFIRMED. **Response:** Mitigate. **Rules:** SC-017, SC-018.

**CT-024 — MFA not enforced for Admins** — **Status:** CONFIRMED. **Rules:** SC-040.

**CT-025 — Admins reset passwords directly** — no reset-code flow. **Status:** CONFIRMED.
**Rules:** SC-042.

**CT-026 — No per-address login limit** — only per username and address. **Status:** CONFIRMED.
**Rules:** SC-043.

### Request handling

**CT-013 — CSRF check skipped without browser headers** — `csrfProtection` passes requests that
carry neither `Origin` nor `Sec-Fetch-Site`. **Status:** CONFIRMED. **Rules:** SC-019.

**CT-014 — No HTTP server timeouts** — `http.ListenAndServe` in `cmd/server/main.go`; also leaves
GO-2026-6089 (HTTP/2 check without `ReadHeaderTimeout`) more exposed. **Status:** CONFIRMED.
**Rules:** SC-026.

**CT-015 — No visitor limit on Rising-Gods lookups and registration requests.** **Status:**
CONFIRMED. **Rules:** SC-028.

**CT-021 — WebAuthn origin falls back to forwarded headers** — production starts without
`PUBLIC_ORIGIN`; `webAuthnRelyingParty` then trusts `X-Forwarded-Host`. **Status:** CONFIRMED.
**Rules:** SC-007.

**CT-027 — Client address from the first `X-Forwarded-For` entry** — bypasses the login limit.
**Status:** FIX PENDING (PR #44). **Rules:** SC-006.

### Data, logging, and privacy

**CT-016 — Security records never deleted** — `auth.Repo.DeleteExpired` is never called; login
attempts, sessions, and MFA challenges accumulate. **Status:** CONFIRMED. **Rules:** SC-038.

**CT-020 — Class icons hotlinked** — `ClassEmblem.tsx` loads from `wow.zamimg.com`. **Status:**
CONFIRMED. **Rules:** SC-024.

**CT-023 — No security event logging** — no log calls in `internal/auth`. **Status:** CONFIRMED.
**Rules:** SC-041.

### Build, deploy, and repository

**CT-017 — Host scripts not in the repository** — `.gitignore` line `bin/` excludes `deploy/bin/`.
**Status:** CONFIRMED. **Rules:** SC-037.

**CT-018 — `main` unprotected** — GitHub API: `protected: false`; ruleset "Protect" disabled.
**Status:** CONFIRMED. **Rules:** SC-039.

**CT-019 — Actions pinned by tag; no Dependabot** — `.github/workflows/*.yml`. **Status:**
CONFIRMED. **Rules:** SC-036.

**CT-022 — Over-long passwords reach bcrypt** — `ValidatePassword` checks only the minimum; bcrypt
then fails and the error surfaces through CT-007. **Status:** CONFIRMED. **Rules:** SC-005.

## Checked without a finding

- **SQL injection:** all queries use Bun builders or `?` binding; the one dynamic column
  (`internal/recruitment/service.go:144`) is chosen from two constants; raw catalog queries are
  constant strings.
- **Broken object-level authorisation:** item locks (owner and lock owner checked), wishlist
  writes, signup save and withdraw, absences (others only for staff), security-key removal
  (credential owner checked), claim codes (bound to the requesting player), registration codes
  (purpose and requester checked) — all enforced in the domain service.
- **Mass assignment:** DTOs with `DisallowUnknownFields`; character edits cannot set the owner.
- **Secrets in responses:** password hashes, token and code hashes, TOTP secrets, WebAuthn data,
  and raw Rising-Gods JSON are tagged `json:"-"`; audit snapshots of registration requests omit
  codes.
- **Client-side injection:** no `dangerouslySetInnerHTML`, `innerHTML`, or `eval`; link targets
  are built by the server (`profileURL`) or from encoded IDs; CSP blocks `javascript:` URLs.
- **Parser crashes:** RS1/RS2 parsers check field counts before indexing.
- **Server-side request forgery:** outbound requests go only to the fixed Rising-Gods origin or the
  configured Worker.
- **CI:** `permissions: contents: read`; `pull_request`, not `pull_request_target`; the Cloudflare
  token is used only on `main` pushes.

## OWASP Top 10 checklist

| OWASP Top 10:2021 | Code threats |
| --- | --- |
| A01 Broken Access Control | CT-006, CT-027 (none in object-level checks) |
| A02 Cryptographic Failures | CT-011, CT-008 |
| A03 Injection | none found |
| A04 Insecure Design | CT-005 |
| A05 Security Misconfiguration | CT-007, CT-013, CT-014, CT-018, CT-021 |
| A06 Vulnerable and Outdated Components | CT-001, CT-019 |
| A07 Identification and Authentication Failures | CT-003, CT-004, CT-009, CT-010, CT-012, CT-022, CT-024 to CT-026 |
| A08 Software and Data Integrity Failures | CT-017, CT-018, CT-019 |
| A09 Security Logging and Monitoring Failures | CT-023 |
| A10 Server-Side Request Forgery | none found |
| Privacy (not in the Top 10) | CT-016, CT-020 |

## Coverage

| Code threat | Status | Response | Rule | Spec threat |
| --- | --- | --- | --- | --- |
| CT-001 | CONFIRMED | Mitigate | SC-045, SC-039 | T-046 |
| CT-003 | CONFIRMED | Mitigate | SC-046 | T-042 |
| CT-004 | CONFIRMED | Mitigate | SC-009 | T-004 |
| CT-005 | DECISION | — | OD-016 | T-043 |
| CT-006 | CONFIRMED | Mitigate | SC-047 | T-044 |
| CT-007 | CONFIRMED | Mitigate | SC-023 | — |
| CT-008 | CONFIRMED | Mitigate | SC-048 | T-045 |
| CT-009 | CONFIRMED | Mitigate | SC-011 | T-005 |
| CT-010 | CONFIRMED | Mitigate | SC-012 | T-006 |
| CT-011 | CONFIRMED | Mitigate | SC-014 | T-011 |
| CT-012 | CONFIRMED | Mitigate | SC-017, SC-018 | T-010 |
| CT-013 | CONFIRMED | Mitigate | SC-019 | T-007 |
| CT-014 | CONFIRMED | Mitigate | SC-026 | T-017 |
| CT-015 | CONFIRMED | Mitigate | SC-028 | T-016, T-035 |
| CT-016 | CONFIRMED | Eliminate | SC-038 | T-022 |
| CT-017 | CONFIRMED | Mitigate | SC-037 | T-013 |
| CT-018 | CONFIRMED | Mitigate | SC-039 | T-012 |
| CT-019 | CONFIRMED | Mitigate | SC-036 | T-014 |
| CT-020 | CONFIRMED | Eliminate | SC-024 | T-024 |
| CT-021 | CONFIRMED | Mitigate | SC-007 | T-008 |
| CT-022 | CONFIRMED | Mitigate | SC-005 | — |
| CT-023 | CONFIRMED | Mitigate | SC-041 | T-021 |
| CT-024 | CONFIRMED | Mitigate | SC-040 | T-020 |
| CT-025 | CONFIRMED | Mitigate | SC-042 | T-026 |
| CT-026 | CONFIRMED | Mitigate | SC-043 | T-003 |
| CT-027 | FIX PENDING | Mitigate | SC-006 | T-001 |

CT-002 found nothing and has no row.

## Limits

- Domain services were reviewed for authorisation and the listed classes, not line by line for
  every business rule.
- The installed production host, its Go version, and the missing `deploy/bin/` scripts could not be
  inspected.
- Who has push access to the repository could not be read.
- No dynamic testing (DAST, fuzzing) was run.
