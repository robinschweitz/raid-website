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
- **Trust boundaries:** TB-1 to TB-7 in `ARCHITECTURE.md`. Threats: `THREAT_MODEL.md` (specification, T-001 to T-050) and `THREAT_MODEL_CODE.md` (implementation, CT-001 to CT-030).
- **Material gaps:** off-server backup storage and key holder (OQ-14, OD-007).
- **Conflicts found:** the client address can be forged behind nginx (SC-006, fixed in PR #44);
  production does not require `PUBLIC_ORIGIN` although WebAuthn depends on it (SC-007); `main` is
  not protected although ADR 071/072 require it (SC-039, T-012); the deploy scripts that
  `deploy/README.md` installs are not in the repository (SC-037, T-013).

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
- **Evidence:** FR-1.2, FR-1.3, FR-27.2, ADR 067. Code: compliant for existing endpoints. Threats: T-023.
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
- **Evidence:** FR-2.3, FR-7.6, ADR 046. Code: **not compliant** on `main` — it returns the first `X-Forwarded-For` entry, so any visitor can choose their own address and bypass the login rate limit. Fixed in PR #44. Threats: T-001.
- **Verification:** Six failed logins with six different forged `X-Forwarded-For` values from one connection are blocked at the sixth.

**SC-007 - WebAuthn relying party from configuration**
- **Status:** ACTIVE
- **Rule:** In production, startup MUST fail without a valid HTTPS `PUBLIC_ORIGIN`, and the WebAuthn RP ID and origin MUST come only from it, never from `Host`, `X-Forwarded-Host`, or `X-Forwarded-Proto`.
- **Enforcement:** `internal/config`; `webAuthnRelyingParty`.
- **Evidence:** ADR 046, `deploy/README.md`. Code: **not compliant** — production starts without `PUBLIC_ORIGIN`, and the fallback trusts forwarded headers that nginx does not overwrite. Threats: T-008.
- **Verification:** `ENV=production` without `PUBLIC_ORIGIN` stops startup; a forged `X-Forwarded-Host` does not change the WebAuthn origin.

**SC-008 - Login throttling**
- **Status:** ACTIVE
- **Rule:** After 5 failed logins for the same normalised username and client address within 15 minutes, further attempts MUST be rejected without checking the password. Failures MUST be recorded in the database.
- **Enforcement:** `auth.StartLogin`, `login_attempts` table.
- **Evidence:** FR-2.3. Code: compliant, but bypassable until SC-006 is fixed. Threats: T-001, T-002.
- **Verification:** The sixth attempt in the window returns the rate-limit error even with the correct password.

**SC-009 - No username probing**
- **Status:** ACTIVE
- **Rule:** Login for an unknown username, an account without a password, and a wrong password MUST return the same error and MUST take comparable time (run a bcrypt comparison against a fixed dummy hash when no hash exists).
- **Enforcement:** `auth.StartLogin`.
- **Evidence:** V6. Code: **not compliant** — unknown usernames return without a bcrypt comparison, so response time reveals which usernames exist. Threats: T-003, T-004.
- **Verification:** Median response time for an unknown and a known username differ by less than the bcrypt cost.

**SC-010 - MFA challenge**
- **Status:** ACTIVE
- **Rule:** The MFA challenge of FR-3.5 MUST be a random 32-byte value, stored as a SHA-256 hash, valid for 5 minutes and usable once.
- **Enforcement:** `auth.StartLogin`, `auth.VerifyMFAChallenge`.
- **Evidence:** FR-3.5, FR-3.6. Code: compliant. Threats: T-005.
- **Verification:** A challenge older than 5 minutes or already used is rejected; no `raid_session` cookie before the second factor.

**SC-011 - MFA guess limit**
- **Status:** ACTIVE
- **Rule:** One MFA challenge MUST accept at most 5 wrong TOTP or recovery codes. The fifth wrong code MUST invalidate the challenge, so the user starts again at the password step, which SC-008 limits.
- **Enforcement:** `auth.VerifyMFAChallenge`; the challenge row counts failures.
- **Evidence:** FR-3.9, V6, OD-002 (owner, 2026-10-22). Code: **not compliant** — no limit today. Threats: T-005.
- **Verification:** After 5 wrong codes, the correct code for that challenge is rejected and a new password login is needed.

**SC-012 - TOTP codes used once**
- **Status:** ACTIVE
- **Rule:** TOTP MUST use 6 digits, 30-second steps, and at most ±1 step of drift, and a code that was already accepted for an account MUST NOT be accepted again within its validity window.
- **Enforcement:** `auth.verifyTOTP`; the MFA settings must store the last accepted time step.
- **Evidence:** FR-3.2, V6. Code: **not compliant** for reuse — no last-used step is stored. Threats: T-006.
- **Verification:** Logging in twice with the same code in the same 30 seconds fails the second time.

**SC-013 - Recovery and registration codes**
- **Status:** ACTIVE
- **Rule:** Recovery codes (10 per setup, 64 random bits) and registration codes (60 random bits, 14-day expiry, revocable) MUST come from `crypto/rand`, MUST be stored only as hashes, MUST be shown once, and MUST be single-use.
- **Enforcement:** `auth.newRecoveryCodes`, `registration.newCode`.
- **Evidence:** FR-3.3, FR-4.4, ADR 040. Code: compliant. Threats: T-025.
- **Verification:** The database holds no plaintext code; a second redemption fails.

**SC-014 - TOTP secrets encrypted at rest**
- **Status:** ACTIVE
- **Rule:** TOTP secrets MUST be stored encrypted with a 256-bit key that comes only from the environment file (`TOTP_ENCRYPTION_KEY`), never from the database or repository. Algorithm: AES-256-GCM from the Go standard library (owner, 2026-10-23, backlog #47) with a random nonce per secret and the player ID as additional authenticated data, so a value copied to another row does not decrypt. With TOTP in use, production MUST NOT start without the key.
- **Stored format:** `v1:<key-id>:<base64 nonce and ciphertext>`, where `<key-id>` is the first 8 hex characters of the SHA-256 of the key. A value without the `v1:` prefix is plaintext.
- **Startup migration:** At startup, before the server accepts requests, every value that is plaintext or uses a key other than the current one MUST be re-encrypted with the current key, all in one database transaction. If any row fails, the transaction MUST roll back and the server MUST exit with an error that names the count of failed rows but no secret. The migration MUST be safe to run again: it touches only rows not yet under the current key. After it, the server MUST check that no such row remains and MUST refuse to start otherwise.
- **Key rotation:** `TOTP_ENCRYPTION_KEY` is the current key; `TOTP_ENCRYPTION_KEYS_PREVIOUS` MAY hold earlier keys, comma-separated. Decryption picks the key by `<key-id>`. A stored value whose key is in neither variable MUST stop startup. To rotate, the operator sets the new key, moves the old one to the previous list, and restarts; the migration re-encrypts every row; the old key MAY be removed after the next successful start. Old database snapshots still need the old key, so it MUST be kept with the backup key material until the last snapshot that uses it has expired (NFR-2.2, SC-034).
- **Enforcement:** `auth` MFA settings repository; `internal/config`.
- **Evidence:** V11, V14, OD-003 (owner, 2026-10-22). Code: **not compliant** — stored as plaintext. Threats: T-011.
- **Verification:** The `totp_secret` column holds no base32 secret; a login with a valid code still works; startup without the key fails while a TOTP setup exists; a migration that fails on one row leaves every row unchanged and the server stopped, and a second start completes it; after a rotation every row carries the new key ID and logins still work; a value copied to another player's row fails to decrypt.

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
- **Evidence:** FR-2.4–2.6, ADR 010. Code: compliant. Threats: T-009.
- **Verification:** Cookie attributes in an HTTP test; the database holds no plaintext token; a logged-out cookie returns 401.

**SC-017 - Ending other sessions**
- **Status:** ACTIVE
- **Rule:** The events in FR-2.13, FR-2.14, FR-5.4, and FR-30.2 MUST delete the affected session rows in the same transaction as the change. Disabling MFA does not end sessions.
- **Enforcement:** `auth.ChangeOwnPassword`, the Admin account write in `registration`/`auth`, `accountmerge`.
- **Evidence:** FR-2.13, FR-2.14, FR-5.4, FR-30.2, OD-001 (owner, 2026-10-22). Code: merge compliant; password and Admin changes **not compliant**. Threats: T-010.
- **Verification:** After a password change, a second session cookie returns 401 and the current one still works.

**SC-018 - Session lifetime**
- **Status:** ACTIVE
- **Rule:** A session MUST end 30 days after login at the latest, and MUST end earlier when it has not been used for 7 days. Each authenticated request resets the idle clock; the last-use time MUST be written at most once per hour, so that ordinary reads do not write to the database.
- **Enforcement:** `auth` session validation; a last-used timestamp on `sessions`.
- **Evidence:** FR-2.4, FR-2.12, V7, OD-008 (owner, 2026-10-22). Code: 30-day maximum compliant; idle timeout **not built**; `last_seen_at` is written on every request (`THREAT_MODEL_CODE.md` CT-028). Threats: T-010.
- **Verification:** A session unused for 7 days and 1 minute returns 401; a session used daily stays valid until day 30.

**SC-019 - CSRF protection**
- **Status:** ACTIVE
- **Rule:** Every state-changing request (not GET, HEAD, OPTIONS) MUST carry an `X-CSRF-Token` header equal to the CSRF cookie, compared in constant time, whether or not it carries a session cookie, `Origin`, or `Sec-Fetch-Site`. This includes login, MFA verification, and registration. The token MUST be 32 bytes from `crypto/rand`.
- **Enforcement:** `csrfProtection` middleware; `GET /api/v1/auth/csrf`; `web/src/api.ts`.
- **Evidence:** FR-2.8, OD-009 (owner, 2026-10-22). Code: **not compliant** — requests without `Origin` and `Sec-Fetch-Site` skip the check. `web/src/api.ts` already sends the token on every non-GET request. Threats: T-007.
- **Verification:** A POST with the session cookie, no CSRF header, and no `Origin` returns 403 `csrf_required`.

### Input, output, and API (V1, V2, V4)

**SC-020 - Strict request decoding**
- **Status:** ACTIVE
- **Rule:** JSON bodies MUST be decoded with `DisallowUnknownFields` and a 1 MiB limit; path IDs MUST be positive integers; enums, roles, classes, specs, statuses, and recruitment slot values MUST be validated on the server.
- **Enforcement:** `DecodeJSON`, `pathID`, `internal/validation`, domain services.
- **Evidence:** NFR-4.1, FR-23.4. Code: compliant except recruitment slot values (FR-23.4, not built). Threats: T-018, T-027.
- **Verification:** Unknown fields, a 1 MiB + 1 body, ID `0`, and an unknown enum each return 4xx.

**SC-021 - Parameterised SQL only**
- **Status:** ACTIVE
- **Rule:** SQL MUST use Bun query builders or `?` parameter binding; user input MUST NOT be concatenated into SQL. Queries live in `repo.go` (DR-4).
- **Enforcement:** repositories.
- **Evidence:** ADR 002, V1. Code: compliant for binding; four files outside `repo.go` (DR-4). Threats: T-027.
- **Verification:** Review of every `NewRaw`, `Where(` with formatted strings, and `ExecContext` call.

**SC-022 - Text-only rendering**
- **Status:** ACTIVE
- **Rule:** The client MUST render all user, addon, and Rising-Gods data as React text; `dangerouslySetInnerHTML`, `innerHTML`, and `eval` MUST NOT be used. Text written into addon formats MUST replace the format separators (`;` → `,`, `|` → `/`).
- **Enforcement:** `web/src`; `internal/addonsync`, `internal/wishlist` exporters.
- **Evidence:** ADR 070, TB-3, TB-5. Code: compliant. Threats: T-009, T-018.
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
- **Evidence:** ADR 046, `DESIGN.md`. Code: compliant; `img-src` still allows `https://wow.zamimg.com`; `style-src` allows `'unsafe-inline'`. Threats: T-009, T-024, T-027.
- **Verification:** Header assertions in an HTTP test.

**SC-025 - TLS at the edge only**
- **Status:** ACTIVE
- **Rule:** The public entry MUST be nginx with TLS (Let's Encrypt), redirecting HTTP to HTTPS; the Go process MUST listen on loopback only in production.
- **Enforcement:** `deploy/nginx/`, `internal/config` (`LISTEN_ADDR` check).
- **Evidence:** NFR-1.3, ADR 070. Code and templates: compliant. Threats: T-029.
- **Verification:** Startup with a non-loopback `LISTEN_ADDR` and `ENV=production` fails.

**SC-026 - HTTP server limits**
- **Status:** ACTIVE
- **Rule:** The Go server MUST set `ReadHeaderTimeout` 10 s, `ReadTimeout` 30 s, `WriteTimeout` 60 s, and `IdleTimeout` 120 s, and on SIGTERM MUST stop accepting connections and give running requests 25 s to finish (systemd stops the service after 30 s).
- **Enforcement:** `cmd/server/main.go` (`http.Server`, `Shutdown`).
- **Evidence:** NFR-4.2, ARCHITECTURE "Server lifecycle", OD-005 (owner, 2026-10-22). Code: **not compliant** — `http.ListenAndServe` without limits. Threats: T-017.
- **Verification:** A client that sends headers slower than 10 s is disconnected; a request running during `systemctl restart` completes.

### External systems (V4, V12, V13)

**SC-027 - Rising-Gods egress**
- **Status:** ACTIVE
- **Rule:** Outbound requests MUST go only to `https://db.rising-gods.de` or the configured HTTPS Worker origin, with query values escaped, a 20-second timeout, and a 5 MiB response cap. Responses MUST be parsed as untrusted data, and a chosen profile MUST be reloaded and filtered before it is stored.
- **Enforcement:** `internal/registration/rising_gods.go`.
- **Evidence:** FR-7.1–7.4, ADR 044, ADR 048. Code: compliant; pasted URLs are accepted only for host `db.rising-gods.de`. Threats: T-016.
- **Verification:** A pasted URL with another host is rejected without an outbound request.

**SC-028 - Visitor lookup limit**
- **Status:** ACTIVE
- **Rule:** For the requests that FR-7.6 limits, one client address (SC-006) MUST NOT start more than 10 in 10 minutes. Answers served from the cache MUST NOT count. Further requests get HTTP 429 with a stable code. The counter is in memory (DR-9).
- **Enforcement:** `GET /api/v1/external/rising-gods/characters`, `POST /api/v1/auth/registration-requests`; counter in `registration` or the HTTP layer.
- **Evidence:** FR-7.6, AQ-7, OD-004 (owner, 2026-10-22). Code: none. Threats: T-016, T-035.
- **Verification:** The eleventh uncached visitor search within 10 minutes returns 429 without an upstream request; a logged-in user is not limited.

**SC-029 - Worker access**
- **Status:** ACTIVE
- **Rule:** The Worker MUST accept only `GET /` with the shared token header and forward only the query to the fixed origin. The token MUST live only in Cloudflare secrets and the server environment file, never in the repository or logs.
- **Enforcement:** `workers/rising-gods-proxy.js`, `internal/config`.
- **Evidence:** ADR 048, ADR 053, ADR 054. Code: compliant. Threats: T-015.
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
- **Evidence:** FR-26.3, ADR 077. Code: compliant. Threats: T-019.
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
- **Evidence:** NFR-2.3, ADR 070. Depends on **OD-007** (OQ-14). Threats: T-011, T-030.
- **Verification:** A restore from the off-server copy on another machine succeeds every month (NFR-2.5).

### Platform, build, and supply chain (V13, V15)

**SC-035 - Service isolation**
- **Status:** ACTIVE
- **Rule:** The application MUST run as the unprivileged `raidweb` user with the systemd restrictions in `deploy/systemd/raid-website.service` (`NoNewPrivileges`, `ProtectSystem=strict`, empty `CapabilityBoundingSet`, write access only to `/var/lib/raid-website`). Builds MUST run as `raiddeploy`, which may only start the fixed root-owned publish unit. SSH MUST NOT allow passwords.
- **Enforcement:** `deploy/systemd/`, `deploy/sudoers.d/`, `deploy/sshd_config.d/`.
- **Evidence:** ADR 070–073. Templates: compliant. Threats: T-011, T-029.
- **Verification:** `systemd-analyze security raid-website` and a review of the installed units against the templates.

**SC-036 - Dependencies and CI**
- **Status:** ACTIVE
- **Rule:** New Go or npm dependencies need owner approval (DR-11). Builds MUST use the committed lock files (`go.sum`, `npm ci`). CI workflows MUST declare minimal `permissions`. Every GitHub Action MUST be pinned to a full commit SHA with its version in a comment, and Dependabot MUST propose updates for GitHub Actions, Go modules, and npm as pull requests (Owner 2026-10-22, OD-006). Deploys come only from the protected `main` branch.
- **Enforcement:** `.github/workflows/`, `deploy/`.
- **Evidence:** ADR 071, ADR 072, DR-11, OD-006. Code: lock files and permissions compliant; actions are pinned by tag (**not compliant**); no Dependabot configuration. Threats: T-012, T-014, T-028.
- **Verification:** Workflow review; `npm ci` and `go build` fail on a lock-file mismatch.

**SC-037 - Deployment scripts under review**
- **Status:** ACTIVE
- **Rule:** Every script that `deploy/README.md` installs (`raid-website-publish`, `-update`, `-backup`, `-configure-ufw`) MUST be committed under `deploy/bin/` and reviewed like code. `.gitignore` MUST NOT exclude `deploy/bin/`. The installed copies MUST match the committed ones.
- **Enforcement:** `.gitignore`, `deploy/bin/`, `deploy/README.md`.
- **Evidence:** ADR 071, ADR 072; `.gitignore` contains `bin/`, and `deploy/bin/` is missing from `main`. Code: **not compliant**. Threats: T-013, T-029.
- **Verification:** `git ls-files deploy/bin` lists all four scripts; a diff of the installed scripts against the repository is empty.

**SC-038 - Short-lived security records are deleted**
- **Status:** ACTIVE
- **Rule:** Login attempts, failed or successful, MUST be deleted once they are older than the longest login-limit window that uses them (SC-008, SC-043); a successful login MUST NOT be kept as an activity history. Expired sessions and expired or used MFA challenges MUST be deleted. Deletion runs during normal requests or at startup, not as a background job.
- **Enforcement:** `auth` repository (`DeleteExpired` exists but is never called).
- **Evidence:** FR-30.6, ARCHITECTURE "No background jobs". Code: **not compliant** — nothing is ever pruned, and every successful login is stored with its address (`THREAT_MODEL_CODE.md` CT-016, CT-029). Threats: T-022.
- **Verification:** After the window, no login attempt, expired session, or old challenge row remains.

**SC-039 - Protected main branch and CI gates**
- **Status:** ACTIVE
- **Rule:** `main` MUST accept changes only through pull requests whose required CI checks pass; direct pushes, force pushes, and deletion MUST be blocked. No approving review is required (one maintainer). Required checks MUST include the Go and frontend gates and a vulnerability scan: `govulncheck` for Go and `npm audit --omit=dev` for the frontend; a known vulnerability blocks the merge until it is fixed.
- **Enforcement:** GitHub ruleset on `main`; `.github/workflows/ci.yml`.
- **Evidence:** ADR 071, ADR 072, NFR-5.1, OD-011 (owner, 2026-10-23). Code: **not compliant** — ruleset "Protect" disabled, no required checks, no vulnerability scan. Threats: T-012, T-028.
- **Verification:** A direct push to `main` is rejected; a pull request with a failing test or a known vulnerable dependency cannot merge.

**SC-040 - MFA required for Admins**
- **Status:** ACTIVE
- **Rule:** An ADMIN account without enabled TOTP or a security key MUST be sent to MFA setup after login, and every other Admin-only request MUST be rejected with HTTP 403 `mfa_setup_required` until MFA is enabled. This includes the bootstrap Admin and a player who is newly given the ADMIN role. Other roles stay optional (FR-3.1).
- **Enforcement:** `auth` login flow; `RequireRole(ADMIN)` route guard.
- **Evidence:** FR-3.10, OD-012 (owner, 2026-10-23). Code: **not compliant** — not built. Threats: T-020.
- **Verification:** An Admin without MFA gets 403 `mfa_setup_required` on `GET /api/v1/audit-log` and succeeds after enabling TOTP.

**SC-041 - Security event logging**
- **Status:** ACTIVE
- **Rule:** The server MUST write one structured journal line for each failed login, login or lookup rate-limit rejection, wrong MFA code, invalidated MFA challenge, CSRF rejection, password change or reset, Admin change of a player's credentials or role, and MFA enable or disable. Each line MUST contain time, event name, player ID or normalised username, client address (SC-006), and request ID, and MUST NOT contain passwords, codes, or tokens (SC-030). The journal MUST keep these lines for 30 days.
- **Enforcement:** `auth`, `csrfProtection`, rate limiters; journald retention in `deploy/`.
- **Evidence:** FR-26.4 (logins stay out of the audit log), OD-013 (owner, 2026-10-23). Code: **not compliant** — `internal/auth` logs nothing. Threats: T-021.
- **Verification:** Five wrong logins produce five journal lines without the password; journald keeps them 30 days.

**SC-042 - Password reset with an in-game code**
- **Status:** ACTIVE
- **Rule:** A password reset MUST use a single-use reset code that an Admin creates for a player and sends in game to that player's main character, with the same randomness, hashing, and revocation as registration codes (SC-013) and a lifetime of 72 hours. Redeeming it MUST set a new password (SC-005), MUST remove the player's TOTP settings, recovery codes, and security keys, and MUST end all sessions of the player (SC-017), in one audited transaction. An Admin MUST NOT set another player's password directly for a reset. Creating a reset code MUST revoke any earlier unused reset code of the same player. Failed redemptions of reset and registration codes MUST count against the per-address limit of SC-043. Creating and redeeming a reset code MUST be logged as security events (SC-041). The in-game message is the only notification channel; a player who did not ask for the reset notices because their old password stops working (accepted: the site stores no e-mail). A reset of an ADMIN is followed by SC-040.
- **Enforcement:** reset-code workflow in `registration`, on the registration-code infrastructure (owner 2026-10-23, backlog #48); `auth`.
- **Evidence:** FR-5.8, OD-014, OD-018 (owner, 2026-10-23). Code: **not compliant** — not built; Admins set passwords directly. Threats: T-026.
- **Verification:** A redeemed code works once; afterwards old sessions return 401, MFA is off, and the new password logs in; a second code revokes the first; the 31st failed redemption from one address in 15 minutes is rejected; a code older than 72 hours is rejected.

**SC-043 - Login limit per client address**
- **Status:** ACTIVE
- **Rule:** Besides SC-008, one client address (SC-006) MUST NOT make more than 30 login attempts, for any usernames, in 15 minutes; further attempts are rejected with the rate-limit error before any password check. There is no separate per-account limit across addresses (T-002 accepted).
- **Enforcement:** `auth.StartLogin`, `login_attempts`.
- **Evidence:** FR-2.3, OD-010 (owner, 2026-10-23). Code: **not compliant** — only per username and address. Threats: T-002, T-003.
- **Verification:** The 31st login attempt from one address within 15 minutes is rejected without a bcrypt comparison.

**SC-044 - Incident plan and secret rotation**
- **Status:** ACTIVE
- **Rule:** `deploy/README.md` MUST contain an incident plan with these steps: end all sessions (the documented database command that deletes every session row), rotate `ADMIN_PASSWORD`, the Worker token, the Cloudflare API token, and the TOTP key, pause automatic deploys (stop `raid-website-update.timer`), and restore from a verified snapshot. There is no in-app "log everyone out" action. The owner is the only person who acts (T-040 accepted).
- **Enforcement:** `deploy/README.md`.
- **Evidence:** OD-015 (owner, 2026-10-23). Code: **not compliant** — no plan or rotation steps documented. Threats: T-040, T-041.
- **Verification:** Each step was carried out once on a test setup and old session cookies returned 401 afterwards.

**SC-045 - Patched Go toolchain**
- **Status:** ACTIVE
- **Rule:** `go.mod` MUST name a `toolchain` at least as new as the newest Go patch release that fixes every reachable `govulncheck` finding (today `go1.26.10`), so that CI and the production build use it even when the host's Go is older. The vulnerability scan of SC-039 MUST run with that toolchain.
- **Enforcement:** `go.mod`; `.github/workflows/ci.yml`; the host update script (SC-037).
- **Evidence:** `THREAT_MODEL_CODE.md` CT-001: 29 reachable standard-library vulnerabilities with `go1.26.0`, all fixed in `go1.26.10`; `go.mod` has no `toolchain` line. Code: **not compliant**. Threats: T-046.
- **Verification:** `govulncheck ./...` with the toolchain from `go.mod` reports no reachable vulnerability; `go version -m` of the production binary shows that toolchain.

**SC-046 - No account hints before a valid code**
- **Status:** ACTIVE
- **Rule:** Public endpoints that take a registration or reset code MUST validate the code before they reveal anything about accounts; with an invalid code they MUST answer `registration_code_invalid` whatever the username. The username check MUST run inside the code transaction, and a duplicate username there MUST map to `409 username_taken`, not to a server error.
- **Enforcement:** `registration.RedeemNewAccount`; the reset-code flow (SC-042).
- **Evidence:** `THREAT_MODEL_CODE.md` CT-003: the username check runs before the code check. Code: **not compliant**. Threats: T-042.
- **Verification:** With an invalid code, a taken and a free username get the same answer.

**SC-047 - The last Admin stays an Admin**
- **Status:** ACTIVE
- **Rule:** A role change, deactivation, merge, or anonymisation that would leave no active ADMIN MUST be rejected with `409 last_admin`.
- **Enforcement:** `auth.SetCredentials`, `accountmerge`, the anonymise operation.
- **Evidence:** FR-5.9; `THREAT_MODEL_CODE.md` CT-006. Code: **not compliant**. Threats: T-044.
- **Verification:** Demoting the only Admin returns 409 and changes nothing.

**SC-048 - Cloned security keys are rejected**
- **Status:** ACTIVE
- **Rule:** When the WebAuthn library reports a clone warning (the signature counter did not increase), the login MUST be rejected, the event logged (SC-041), and the counter left unchanged.
- **Enforcement:** `auth.FinishWebAuthnMFA`.
- **Evidence:** `THREAT_MODEL_CODE.md` CT-008: the warning is never checked. Code: **not compliant**. Threats: T-045.
- **Verification:** An assertion with a lower counter than stored fails.

**SC-049 - The Admin confirms a match by character name**
- **Status:** ACTIVE
- **Rule:** When a registration request has no character linked to its Rising-Gods profile but a player without a login owns a character with the requested name, the Admin MUST see that player before creating the code and MUST choose either "attach to this player" or "new player". The choice MUST be stored with the code. Redeeming MUST follow the stored choice and MUST NOT match by name on its own.
- **Enforcement:** `registration` code creation and `RedeemNewAccount`; the admin request list.
- **Evidence:** FR-4.5, OD-016 (owner, 2026-10-23); `THREAT_MODEL_CODE.md` CT-005: redeem falls back to `FindByName` without any confirmation. Code: **not compliant**. Threats: T-043.
- **Verification:** A request whose name matches an imported player cannot get a code until the Admin chooses; with "new player" the redeem creates a new player even though the name matches.

**SC-050 - Only the profile fields the site uses are stored**
- **Status:** ACTIVE
- **Rule:** From a Rising-Gods profile the system MUST store only character name, class, specs, and gear score. The raw response MUST NOT be stored; existing raw copies on registration requests and characters MUST be removed by a migration.
- **Enforcement:** `registration` and `characters` repositories; a database migration that drops or empties `raw_profile_json` and `external_profile_json`.
- **Evidence:** FR-30.6, OD-017 (owner, 2026-10-23); `THREAT_MODEL_CODE.md` CT-030: the full response is stored and never read. Code: **not compliant**. Threats: T-047.
- **Verification:** After registration and after the migration, no row holds a raw profile; the four fields are still shown.

**SC-051 - Server-side reset code for a locked-out Admin**
- **Status:** ACTIVE
- **Rule:** A command `cmd/reset-code`, run on the server as the application user, MUST create a reset code (SC-042) for a named player and print it once. It MUST open the database directly, MUST NOT be reachable over HTTP, MUST write an audit entry with the actor "server operator", and MUST be logged as a security event (SC-041). It MUST NOT set a password or remove MFA itself; that happens only when the code is redeemed.
- **Enforcement:** `cmd/reset-code` (new) using the `registration` reset-code service.
- **Evidence:** FR-3.11, OD-019 (owner, 2026-10-23). Code: **not built**. Threats: T-049.
- **Verification:** Running the command for the only Admin prints a code that works once within 72 hours; the audit log shows the operator entry; no HTTP route reaches the command.

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

- **OD-007** DEFERRED. Off-server backup storage and key holder (SC-034). Owner: owner. Done when SC-034's rule and verification are met: an encrypted daily copy off the host, a key that also exists off the host, and a monthly restore test on another machine. Decide by 2026-11-30 (OD-020).
  - Missing: storage location and who holds the encryption key. Same decision as OQ-14 in `REQUIREMENTS.md`.
  - Affected: SC-034, NFR-2.3. Owner: owner.

Decided by the owner on 2026-10-23 (threat model):

- **OD-010** Per-address login limit of 30 attempts in 15 minutes; no per-account limit → SC-043.
- **OD-011** `main` only through pull requests with passing CI and a blocking vulnerability scan; no second approver → SC-039.
- **OD-012** MFA required for Admins → SC-040.
- **OD-013** Security events to the server journal, kept 30 days → SC-041.
- **OD-014** Password reset with an in-game code that also removes MFA → SC-042.
- **OD-015** Written incident plan, no in-app button; the owner stays the only key holder → SC-044.
- **OD-016** A match by character name is attached only after the Admin confirms it when creating the code → SC-049, FR-4.5.
- **OD-017** Only name, class, specs, and gear score are kept from a Rising-Gods profile; raw copies are dropped → SC-050, FR-30.6.
- **OD-018** A password reset code lasts 72 hours → SC-042.
- **OD-019** A locked-out only Admin gets a reset code from a server command → SC-051, FR-3.11.
- **OD-020** OD-007 (off-server backup) is decided by 2026-11-30.
