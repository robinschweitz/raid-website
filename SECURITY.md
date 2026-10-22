# Security — raid-website

This file owns the security posture: secure-coding rules, controls, trust-boundary enforcement,
and open security decisions. All security controls are written here; other files reference the
`SC-###` and `OD-###` IDs. It reflects the code of v1.0.27 and the deployment templates in
`deploy/` as of 2026-10-22. Each rule says in its **Evidence** whether the current code complies.

## Secure-Coding Inputs

- **Read:** `REQUIREMENTS.md`, `DESIGN.md`, `ARCHITECTURE.md`, `AGENTS.md`, `CODING_RULES.md`,
  `deploy/README.md`, `deploy/` templates, `workers/rising-gods-proxy.js`, `.github/workflows/`,
  and the Go and TypeScript sources they describe. No earlier `SECURITY.md` existed.
- **Confirmed facts used:** Go 1.26 `net/http` server behind nginx (TLS, Let's Encrypt) on one
  host; SQLite; React 18 SPA from the same origin; local accounts with bcrypt, optional TOTP,
  recovery codes, and WebAuthn (`go-webauthn/webauthn` v0.18.2); cookie sessions; roles
  GUEST < MEMBER < RAIDASSISTENT < RAIDLEADER < ADMIN; public reads in FR-1.2; outbound HTTPS to
  `db.rising-gods.de`, optionally via a Cloudflare Worker; personal data under GDPR (FR-30);
  production sets `ENV=production`, `COOKIE_SECURE=true`, `TRUST_PROXY_HEADERS=true`,
  `PUBLIC_ORIGIN` (deploy/README.md).
- **Trust boundaries:** TB-1 to TB-7 in `ARCHITECTURE.md`.
- **Material gaps:** off-server backup storage and key holder (OQ-14, OD-007).
- **Conflicts found:** the client address can be forged behind nginx (SC-006, fixed in PR #44);
  production does not require `PUBLIC_ORIGIN` although WebAuthn depends on it (SC-007).

## Selected Manicode Prompts

The Manicode secure-coding corpus that the bootstrap playbook names
(`/Users/jmanico/Dropbox/github/aegis/secure-coding-knowledge`) is private and not available to
this project. No Manicode packs are selected.

Instead, rules are mapped to **OWASP ASVS 5.0.0** chapters (May 2025 release). Mappings are at
chapter level only; requirement IDs are not cited until each is verified against the standard.

- V1 Encoding and Sanitization; V2 Validation and Business Logic; V3 Web Frontend Security;
  V4 API and Web Service; V6 Authentication; V7 Session Management; V8 Authorization;
  V11 Cryptography; V12 Secure Communication; V13 Configuration; V14 Data Protection;
  V15 Secure Coding and Architecture; V16 Security Logging and Error Handling.
- Not applicable: V5 File Handling (no uploads), V9 Self-contained Tokens (opaque tokens only),
  V10 OAuth and OIDC (none), V17 WebRTC (none).

## Secure-Coding Rules

### Authorization (V8)

**SC-001 - Server-side authorization on every route**
- **Status:** ACTIVE
- **Rule:** Every `/api/v1` route MUST be registered with `RequireAuth` or `RequireRole`, except the public reads listed in FR-1.2 and the login, MFA, CSRF, registration, and health endpoints. Hiding a control in the client MUST NOT be the only check.
- **Enforcement:** `internal/httpapi/v1` route registration; domain services for ownership.
- **Evidence:** FR-1.1, FR-1.6, ADR 010, ADR 067. Code: compliant.
- **Verification:** For each mutation, a test with a role one step too low gets 403 and one without a session gets 401 (CODING_RULES §13.2).

**SC-002 - Self-service actions bound to the session actor**
- **Status:** ACTIVE
- **Rule:** The domain service of every self-service write (own locks FR-17.1, own wishlist FR-19.1, own signups and absences FR-12, own vote FR-24.3, own password and MFA FR-2.7, FR-3) MUST compare the target player with the session's player and reject a mismatch, unless a staff or Admin requirement allows it.
- **Enforcement:** `enhancepoints`, `wishlist`, `raidsignup`, `poll`, `auth` services.
- **Evidence:** FR-17.1, FR-19.1, ADR 020. Code: compliant.
- **Verification:** A MEMBER request for another player's lock, wishlist, or absence returns 403 and changes nothing.

**SC-003 - Actor comes only from the session**
- **Status:** ACTIVE
- **Rule:** Handlers MUST take the actor from the request context set by the session middleware and MUST NOT read an actor from headers or bodies. The removed `X-Actor-ID` header MUST NOT return.
- **Enforcement:** `internal/httpapi/v1/actor.go`, `authenticate` middleware.
- **Evidence:** ADR 010, FR-24.6. Code: compliant (poll handlers still mention the header in an error message, FR-24.6).
- **Verification:** Sending `X-Actor-ID` changes nothing; code search finds no header read.

**SC-004 - Public data stays minimal**
- **Status:** ACTIVE
- **Rule:** Only the reads in FR-1.2 may be served without a session (paths: `ARCHITECTURE.md` "Public interface"). Unauthenticated responses MUST be built from dedicated public DTOs that contain only the fields FR-1.3, FR-27.2, and FR-30.1 allow, never from the authenticated DTOs.
- **Enforcement:** DTOs of the public endpoints; `GET /api/v1/statistics` (not built).
- **Evidence:** FR-1.2, FR-1.3, FR-27.2, ADR 067. Code: compliant for existing endpoints.
- **Verification:** HTTP tests compare the exact field set of each public response.

### Authentication (V6)

**SC-005 - Password storage and policy**
- **Status:** ACTIVE
- **Rule:** Password policy: at least 12 characters after trimming. Passwords MUST be hashed with bcrypt (`golang.org/x/crypto/bcrypt`, `DefaultCost`). A password longer than bcrypt's 72-byte input limit MUST be rejected with a validation error, not truncated and not answered with a 500.
- **Enforcement:** `auth.ValidatePassword`, `auth.HashPassword`.
- **Evidence:** FR-2.2, ADR 046. Code: compliant for hashing and minimum; the over-72-byte case is not validated before hashing.
- **Verification:** A 73-byte password gets a 4xx validation error; a 12-character one is accepted; the stored value starts with `$2`.

**SC-006 - Trustworthy client address behind the proxy**
- **Status:** ACTIVE
- **Rule:** With `TRUST_PROXY_HEADERS=true`, the client address MUST come from `X-Real-IP`, which nginx overwrites with `$remote_addr`. The leftmost `X-Forwarded-For` entry MUST NOT be used, because nginx appends to a client-supplied value. Without trusted proxy mode, `RemoteAddr` is used.
- **Enforcement:** `clientAddress` in `internal/httpapi/v1/security.go`; `deploy/nginx/raid-website.conf`.
- **Evidence:** FR-2.3, FR-7.6, ADR 046. Code: **not compliant** on `main` — it returns the first `X-Forwarded-For` entry, so any visitor can choose their own address and bypass the login rate limit. Fixed in PR #44.
- **Verification:** Six failed logins with six different forged `X-Forwarded-For` values from one connection are blocked at the sixth.

**SC-007 - WebAuthn relying party from configuration**
- **Status:** ACTIVE
- **Rule:** In production, startup MUST fail without a valid HTTPS `PUBLIC_ORIGIN`, and the WebAuthn RP ID and origin MUST come only from it, never from `Host`, `X-Forwarded-Host`, or `X-Forwarded-Proto`.
- **Enforcement:** `internal/config`; `webAuthnRelyingParty`.
- **Evidence:** ADR 046, `deploy/README.md`. Code: **not compliant** — production starts without `PUBLIC_ORIGIN`, and the fallback trusts forwarded headers that nginx does not overwrite.
- **Verification:** `ENV=production` without `PUBLIC_ORIGIN` stops startup; a forged `X-Forwarded-Host` does not change the WebAuthn origin.

**SC-008 - Login throttling**
- **Status:** ACTIVE
- **Rule:** After 5 failed logins for the same normalised username and client address within 15 minutes, further attempts MUST be rejected without checking the password. Failures MUST be recorded in the database.
- **Enforcement:** `auth.StartLogin`, `login_attempts` table.
- **Evidence:** FR-2.3. Code: compliant, but bypassable until SC-006 is fixed.
- **Verification:** The sixth attempt in the window returns the rate-limit error even with the correct password.

**SC-009 - No username probing**
- **Status:** ACTIVE
- **Rule:** Login for an unknown username, an account without a password, and a wrong password MUST return the same error and MUST take comparable time (run a bcrypt comparison against a fixed dummy hash when no hash exists).
- **Enforcement:** `auth.StartLogin`.
- **Evidence:** V6. Code: **not compliant** — unknown usernames return without a bcrypt comparison, so response time reveals which usernames exist.
- **Verification:** Median response time for an unknown and a known username differ by less than the bcrypt cost.

**SC-010 - MFA challenge**
- **Status:** ACTIVE
- **Rule:** The MFA challenge of FR-3.5 MUST be a random 32-byte value, stored as a SHA-256 hash, valid for 5 minutes and usable once.
- **Enforcement:** `auth.StartLogin`, `auth.VerifyMFAChallenge`.
- **Evidence:** FR-3.5, FR-3.6. Code: compliant.
- **Verification:** A challenge older than 5 minutes or already used is rejected; no `raid_session` cookie before the second factor.

**SC-011 - MFA guess limit**
- **Status:** ACTIVE
- **Rule:** One MFA challenge MUST accept at most 5 wrong TOTP or recovery codes. The fifth wrong code MUST invalidate the challenge, so the user starts again at the password step, which SC-008 limits.
- **Enforcement:** `auth.VerifyMFAChallenge`; the challenge row counts failures.
- **Evidence:** FR-3.9, V6, OD-002 (owner, 2026-10-22). Code: **not compliant** — no limit today.
- **Verification:** After 5 wrong codes, the correct code for that challenge is rejected and a new password login is needed.

**SC-012 - TOTP codes used once**
- **Status:** ACTIVE
- **Rule:** TOTP MUST use 6 digits, 30-second steps, and at most ±1 step of drift, and a code that was already accepted for an account MUST NOT be accepted again within its validity window.
- **Enforcement:** `auth.verifyTOTP`; the MFA settings must store the last accepted time step.
- **Evidence:** FR-3.2, V6. Code: **not compliant** for reuse — no last-used step is stored.
- **Verification:** Logging in twice with the same code in the same 30 seconds fails the second time.

**SC-013 - Recovery and registration codes**
- **Status:** ACTIVE
- **Rule:** Recovery codes (10 per setup, 64 random bits) and registration codes (60 random bits, 14-day expiry, revocable) MUST come from `crypto/rand`, MUST be stored only as hashes, MUST be shown once, and MUST be single-use.
- **Enforcement:** `auth.newRecoveryCodes`, `registration.newCode`.
- **Evidence:** FR-3.3, FR-4.4, ADR 040. Code: compliant.
- **Verification:** The database holds no plaintext code; a second redemption fails.

**SC-014 - TOTP secrets encrypted at rest**
- **Status:** ACTIVE
- **Rule:** TOTP secrets MUST be stored encrypted with a 256-bit key that comes only from the environment file (`TOTP_ENCRYPTION_KEY`), never from the database or repository. PROPOSED algorithm: AES-256-GCM from the Go standard library with a random nonce per secret. Existing plaintext secrets MUST be encrypted once at startup. With TOTP in use, production MUST NOT start without the key.
- **Enforcement:** `auth` MFA settings repository; `internal/config`.
- **Evidence:** V11, V14, OD-003 (owner, 2026-10-22). Code: **not compliant** — stored as plaintext.
- **Verification:** The `totp_secret` column holds no base32 secret; a login with a valid code still works; startup without the key fails while a TOTP setup exists.

**SC-015 - Re-authentication for security changes**
- **Status:** ACTIVE
- **Rule:** Changing the own password, disabling TOTP, and removing a security key MUST require the current password.
- **Enforcement:** `auth.ChangeOwnPassword`, `auth.DisableTOTP`, `auth.DeleteWebAuthnCredential`.
- **Evidence:** ADR 046. Code: compliant.
- **Verification:** Each request with a wrong current password fails and changes nothing.

### Session management (V7)

**SC-016 - Session token and cookie**
- **Status:** ACTIVE
- **Rule:** Session tokens MUST be 32 bytes from `crypto/rand`, stored only as a SHA-256 hash, and sent as cookie `raid_session` with `HttpOnly`, `SameSite=Lax`, `Path=/`, and `Secure` in production. Each login MUST create a new token; logout MUST delete the session row.
- **Enforcement:** `auth.createSession`, `internal/httpapi/v1/auth.go`.
- **Evidence:** FR-2.4–2.6, ADR 010. Code: compliant.
- **Verification:** Cookie attributes in an HTTP test; the database holds no plaintext token; a logged-out cookie returns 401.

**SC-017 - Ending other sessions**
- **Status:** ACTIVE
- **Rule:** The events in FR-2.13, FR-2.14, FR-5.4, and FR-30.2 MUST delete the affected session rows in the same transaction as the change. Disabling MFA does not end sessions.
- **Enforcement:** `auth.ChangeOwnPassword`, the Admin account write in `registration`/`auth`, `accountmerge`.
- **Evidence:** FR-2.13, FR-2.14, FR-5.4, FR-30.2, OD-001 (owner, 2026-10-22). Code: merge compliant; password and Admin changes **not compliant**.
- **Verification:** After a password change, a second session cookie returns 401 and the current one still works.

**SC-018 - Session lifetime**
- **Status:** ACTIVE
- **Rule:** A session MUST end 30 days after login at the latest, and MUST end earlier when it has not been used for 7 days. Each authenticated request resets the idle clock; updating the last-use time at most once per hour is enough.
- **Enforcement:** `auth` session validation; a last-used timestamp on `sessions`.
- **Evidence:** FR-2.4, FR-2.12, V7, OD-008 (owner, 2026-10-22). Code: 30-day maximum compliant; idle timeout **not built**.
- **Verification:** A session unused for 7 days and 1 minute returns 401; a session used daily stays valid until day 30.

**SC-019 - CSRF protection**
- **Status:** ACTIVE
- **Rule:** Every state-changing request (not GET, HEAD, OPTIONS) MUST carry an `X-CSRF-Token` header equal to the CSRF cookie, compared in constant time, whether or not it carries a session cookie, `Origin`, or `Sec-Fetch-Site`. This includes login, MFA verification, and registration. The token MUST be 32 bytes from `crypto/rand`.
- **Enforcement:** `csrfProtection` middleware; `GET /api/v1/auth/csrf`; `web/src/api.ts`.
- **Evidence:** FR-2.8, OD-009 (owner, 2026-10-22). Code: **not compliant** — requests without `Origin` and `Sec-Fetch-Site` skip the check. `web/src/api.ts` already sends the token on every non-GET request.
- **Verification:** A POST with the session cookie, no CSRF header, and no `Origin` returns 403 `csrf_required`.

### Input, output, and API (V1, V2, V4)

**SC-020 - Strict request decoding**
- **Status:** ACTIVE
- **Rule:** JSON bodies MUST be decoded with `DisallowUnknownFields` and a 1 MiB limit; path IDs MUST be positive integers; enums, roles, classes, specs, statuses, and recruitment slot values MUST be validated on the server.
- **Enforcement:** `DecodeJSON`, `pathID`, `internal/validation`, domain services.
- **Evidence:** NFR-4.1, FR-23.4. Code: compliant except recruitment slot values (FR-23.4, not built).
- **Verification:** Unknown fields, a 1 MiB + 1 body, ID `0`, and an unknown enum each return 4xx.

**SC-021 - Parameterised SQL only**
- **Status:** ACTIVE
- **Rule:** SQL MUST use Bun query builders or `?` parameter binding; user input MUST NOT be concatenated into SQL. Queries live in `repo.go` (DR-4).
- **Enforcement:** repositories.
- **Evidence:** ADR 002, V1. Code: compliant for binding; four files outside `repo.go` (DR-4).
- **Verification:** Review of every `NewRaw`, `Where(` with formatted strings, and `ExecContext` call.

**SC-022 - Text-only rendering**
- **Status:** ACTIVE
- **Rule:** The client MUST render all user, addon, and Rising-Gods data as React text; `dangerouslySetInnerHTML`, `innerHTML`, and `eval` MUST NOT be used. Text written into addon formats MUST replace the format separators (`;` → `,`, `|` → `/`).
- **Enforcement:** `web/src`; `internal/addonsync`, `internal/wishlist` exporters.
- **Evidence:** ADR 070, TB-3, TB-5. Code: compliant.
- **Verification:** Code search finds none of the forbidden APIs; a name containing `;` round-trips without adding a field.

**SC-023 - Safe errors**
- **Status:** ACTIVE
- **Rule:** Error responses MUST use the `{"error": {"code", "message"}}` envelope with a stable code. 5xx messages MUST be generic and MUST NOT contain error chains, SQL, file paths, stack traces, or upstream responses.
- **Enforcement:** `WriteError` call sites in `internal/httpapi/v1`.
- **Evidence:** FR-7.5, ADR 053. Code: compliant; some 4xx messages pass domain sentinel texts (`err.Error()`), which contain no internal data.
- **Verification:** Forced repository failure returns `internal` with a generic message.

### Web frontend and transport (V3, V12)

**SC-024 - Security headers**
- **Status:** ACTIVE
- **Rule:** Every response MUST carry `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`, `X-Frame-Options: DENY`, the restrictive `Permissions-Policy`, and a CSP with `default-src 'self'`, `script-src 'self'`, `object-src 'none'`, `frame-ancestors 'none'`, `base-uri 'self'`, `form-action 'self'`, `connect-src 'self'`. `img-src` MUST be `'self' data:` once the class icons are local (DQ-2). HSTS (`max-age=31536000; includeSubDomains`) MUST be sent in production.
- **Enforcement:** `securityHeaders` middleware.
- **Evidence:** ADR 046, `DESIGN.md`. Code: compliant; `img-src` still allows `https://wow.zamimg.com`; `style-src` allows `'unsafe-inline'`.
- **Verification:** Header assertions in an HTTP test.

**SC-025 - TLS at the edge only**
- **Status:** ACTIVE
- **Rule:** The public entry MUST be nginx with TLS (Let's Encrypt), redirecting HTTP to HTTPS; the Go process MUST listen on loopback only in production.
- **Enforcement:** `deploy/nginx/`, `internal/config` (`LISTEN_ADDR` check).
- **Evidence:** NFR-1.3, ADR 070. Code and templates: compliant.
- **Verification:** Startup with a non-loopback `LISTEN_ADDR` and `ENV=production` fails.

**SC-026 - HTTP server limits**
- **Status:** ACTIVE
- **Rule:** The Go server MUST set `ReadHeaderTimeout` 10 s, `ReadTimeout` 30 s, `WriteTimeout` 60 s, and `IdleTimeout` 120 s, and on SIGTERM MUST stop accepting connections and give running requests 25 s to finish (systemd stops the service after 30 s).
- **Enforcement:** `cmd/server/main.go` (`http.Server`, `Shutdown`).
- **Evidence:** NFR-4.2, ARCHITECTURE "Server lifecycle", OD-005 (owner, 2026-10-22). Code: **not compliant** — `http.ListenAndServe` without limits.
- **Verification:** A client that sends headers slower than 10 s is disconnected; a request running during `systemctl restart` completes.

### External systems (V4, V12, V13)

**SC-027 - Rising-Gods egress**
- **Status:** ACTIVE
- **Rule:** Outbound requests MUST go only to `https://db.rising-gods.de` or the configured HTTPS Worker origin, with query values escaped, a 20-second timeout, and a 5 MiB response cap. Responses MUST be parsed as untrusted data, and a chosen profile MUST be reloaded and filtered before it is stored.
- **Enforcement:** `internal/registration/rising_gods.go`.
- **Evidence:** FR-7.1–7.4, ADR 044, ADR 048. Code: compliant; pasted URLs are accepted only for host `db.rising-gods.de`.
- **Verification:** A pasted URL with another host is rejected without an outbound request.

**SC-028 - Visitor lookup limit**
- **Status:** ACTIVE
- **Rule:** For the requests that FR-7.6 limits, one client address (SC-006) MUST NOT start more than 10 in 10 minutes. Answers served from the cache MUST NOT count. Further requests get HTTP 429 with a stable code. The counter is in memory (DR-9).
- **Enforcement:** `GET /api/v1/external/rising-gods/characters`, `POST /api/v1/auth/registration-requests`; counter in `registration` or the HTTP layer.
- **Evidence:** FR-7.6, AQ-7, OD-004 (owner, 2026-10-22). Code: none.
- **Verification:** The eleventh uncached visitor search within 10 minutes returns 429 without an upstream request; a logged-in user is not limited.

**SC-029 - Worker access**
- **Status:** ACTIVE
- **Rule:** The Worker MUST accept only `GET /` with the shared token header and forward only the query to the fixed origin. The token MUST live only in Cloudflare secrets and the server environment file, never in the repository or logs.
- **Enforcement:** `workers/rising-gods-proxy.js`, `internal/config`.
- **Evidence:** ADR 048, ADR 053, ADR 054. Code: compliant.
- **Verification:** Requests without the token or with another method or path are rejected.

### Data protection and logging (V14, V16)

**SC-030 - Logs without secrets**
- **Status:** ACTIVE
- **Rule:** Request logs MUST contain only method, path without query, status, duration, and request ID. Bodies, cookies, tokens, passwords, codes, and query strings MUST NOT be logged.
- **Enforcement:** `logging` middleware; every `log.Printf` call.
- **Evidence:** FR-26.3. Code: compliant.
- **Verification:** Code review of log calls; a login request leaves no password or token in the journal.

**SC-031 - Audit snapshots without secrets**
- **Status:** ACTIVE
- **Rule:** Audit before and after snapshots (FR-26.3) MUST NOT contain passwords, password hashes, TOTP secrets, recovery codes, registration codes, or raw Rising-Gods profile data, and MUST record account changes only as "password changed: yes/no".
- **Enforcement:** `audit` writers in each domain.
- **Evidence:** FR-26.3, ADR 077. Code: compliant.
- **Verification:** `audit_coverage_test` checks the account snapshot for the password and hash; the same check is needed for MFA, registration-code, and Rising-Gods snapshots.

**SC-032 - Anonymisation**
- **Status:** ACTIVE
- **Rule:** Anonymisation (FR-30.2–30.4) MUST run in one transaction and MUST remove every credential row of the player: password hash, sessions, MFA settings, recovery codes, and WebAuthn credentials (SC-017).
- **Enforcement:** `accountmerge` anonymise operation (not built).
- **Evidence:** FR-30, AQ-6. Code: not built.
- **Verification:** After anonymisation no row links the player to a login, secret, or external profile; old sessions return 401.

**SC-033 - Secrets in configuration**
- **Status:** ACTIVE
- **Rule:** `ADMIN_PASSWORD`, `RISING_GODS_PROXY_TOKEN`, and the privacy contact values MUST come only from `/etc/raid-website.env` (readable by root and the service only) or Cloudflare secrets, never from the repository. Production MUST NOT start with default admin credentials.
- **Enforcement:** `internal/config`, `deploy/`.
- **Evidence:** NFR-1.2, FR-30.1, ADR 020, ADR 048. Code: compliant.
- **Verification:** A secret scan of the repository finds none; production with default credentials fails startup.

**SC-034 - Off-server backups**
- **Status:** BLOCKED
- **Rule:** The daily off-server copy MUST be encrypted before it leaves the host, and the key MUST NOT exist only on the host (NFR-2.3). Blocked on storage and key holder.
- **Enforcement:** `deploy/` backup units.
- **Evidence:** NFR-2.3, ADR 070. Depends on **OD-007** (OQ-14).
- **Verification:** A restore from the off-server copy on another machine succeeds every month (NFR-2.5).

### Platform, build, and supply chain (V13, V15)

**SC-035 - Service isolation**
- **Status:** ACTIVE
- **Rule:** The application MUST run as the unprivileged `raidweb` user with the systemd restrictions in `deploy/systemd/raid-website.service` (`NoNewPrivileges`, `ProtectSystem=strict`, empty `CapabilityBoundingSet`, write access only to `/var/lib/raid-website`). Builds MUST run as `raiddeploy`, which may only start the fixed root-owned publish unit. SSH MUST NOT allow passwords.
- **Enforcement:** `deploy/systemd/`, `deploy/sudoers.d/`, `deploy/sshd_config.d/`.
- **Evidence:** ADR 070–073. Templates: compliant.
- **Verification:** `systemd-analyze security raid-website` and a review of the installed units against the templates.

**SC-036 - Dependencies and CI**
- **Status:** ACTIVE
- **Rule:** New Go or npm dependencies need owner approval (DR-11). Builds MUST use the committed lock files (`go.sum`, `npm ci`). CI workflows MUST declare minimal `permissions`. Every GitHub Action MUST be pinned to a full commit SHA with its version in a comment, and Dependabot MUST propose updates for GitHub Actions, Go modules, and npm as pull requests (Owner 2026-10-22, OD-006). Deploys come only from the protected `main` branch.
- **Enforcement:** `.github/workflows/`, `deploy/`.
- **Evidence:** ADR 071, ADR 072, DR-11, OD-006. Code: lock files and permissions compliant; actions are pinned by tag (**not compliant**); no Dependabot configuration.
- **Verification:** Workflow review; `npm ci` and `go build` fail on a lock-file mismatch.

## Open Security Decisions

The owner decided OD-001 to OD-006, OD-008, and OD-009 on 2026-10-22; the answers are in the rules that
cite them. IDs are not reused.

- **OD-001** Own password change and Admin account changes end other sessions → SC-017.
- **OD-002** 5 wrong MFA codes per challenge → SC-011.
- **OD-003** TOTP secrets encrypted with a key from the environment file → SC-014.
- **OD-004** 10 Rising-Gods-reaching visitor requests per 10 minutes → SC-028.
- **OD-005** Timeouts 10 / 30 / 60 / 120 s, 25 s shutdown → SC-026.
- **OD-006** Actions pinned by SHA, Dependabot → SC-036.
- **OD-008** 7-day idle timeout → SC-018.
- **OD-009** CSRF token also for requests without a session cookie (login, MFA, registration) → SC-019, FR-2.8. Found during ownership reconciliation.

- **OD-007** DEFERRED. Off-server backup storage and key holder (SC-034).
  - Missing: storage location and who holds the encryption key. Same decision as OQ-14 in `REQUIREMENTS.md`.
  - Affected: SC-034, NFR-2.3. Owner: owner.

