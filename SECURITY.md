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
  host; SQLite; React 18 SPA from the same origin; local accounts with Argon2id password hashes (legacy bcrypt until the next login), optional TOTP,
  recovery codes, and WebAuthn (`go-webauthn/webauthn` v0.18.2); cookie sessions; roles
  GUEST < MEMBER < RAIDASSISTENT < RAIDLEADER < ADMIN; public reads in FR-1.2; outbound HTTPS to
  `db.rising-gods.de`, optionally via a Cloudflare Worker; personal data under GDPR (FR-30);
  production sets `ENV=production`, `COOKIE_SECURE=true`, `TRUST_PROXY_HEADERS=true`,
  `PUBLIC_ORIGIN` (deploy/README.md).
- **Trust boundaries:** TB-1 to TB-7 in `ARCHITECTURE.md`. Threats: `THREAT_MODEL.md` (specification, T-001 to T-050) and `THREAT_MODEL_CODE.md` (implementation, CT-001 to CT-030).
- **Material gaps:** none; the off-server backup storage and key holder were decided on 2026-10-24 (OD-007, ADR 099).
- **Conflicts found:** the client address can be forged behind nginx (SC-006, fixed in PR #44);
  production did not require `PUBLIC_ORIGIN` although WebAuthn depends on it (SC-007, fixed #71); `main` was
  not protected although ADR 071/072 require it (SC-039, T-012, fixed 2026-10-23); the deploy
  scripts that `deploy/README.md` installs were not in the repository (SC-037, T-013, fixed #105).

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
- **Evidence:** ADR 010, FR-24.6. Code: compliant. Poll writes answer a missing session with `unauthenticated` (ADR 095) and no longer mention the header (`internal/httpapi/v1/polls.go`; test `TestReqRaid013_AC01`).
- **Verification:** Sending `X-Actor-ID` changes nothing; code search finds no header read.

**SC-004 - Public data stays minimal**
- **Status:** ACTIVE
- **Rule:** Only the reads in FR-1.2 may be served without a session (paths: `ARCHITECTURE.md` "Public interface"). Unauthenticated responses MUST be built from dedicated public DTOs that contain only the fields FR-1.3, FR-27.2, and FR-30.1 allow, never from the authenticated DTOs.
- **Enforcement:** DTOs of the public endpoints; `GET /api/v1/statistics`.
- **Evidence:** FR-1.2, FR-1.3, FR-27.2, ADR 067. Code: compliant for existing endpoints. Threats: T-023.
- **Verification:** HTTP tests compare the exact field set of each public response.

### Authentication (V6)

**SC-005 - Password storage and policy**
- **Status:** ACTIVE
- **Rule:** Password policy: at least 12 characters after trimming and at most 1024 bytes. A longer password MUST be rejected with a validation error, never truncated. New and changed passwords MUST be hashed with Argon2id (`golang.org/x/crypto/argon2`, 64 MiB, 3 passes, 1 lane, 16-byte random salt, PHC string). Existing bcrypt hashes MUST keep working, and after a successful login MUST be replaced by an Argon2id hash of the same password; the replacement MUST NOT overwrite a hash that changed meanwhile. At most two Argon2id computations MAY run at once, and stored parameters outside fixed bounds MUST be rejected before hashing. A request MUST NOT wait longer than 5 seconds for a free hashing slot, and MUST stop waiting when the request ends; then it MUST answer `503 server_busy` with `Retry-After`, MUST NOT count as a failed login, and an unknown username MUST get the same answer (SC-009) (Owner 2026-10-25, ADR 103).
- **Enforcement:** `auth.ValidatePassword`, `auth.HashPassword`, `auth.verifyStoredPassword`, `auth.StartLogin` (rehash), `auth.Repo.UpgradePasswordHash`; `argonSlotWait`, `ErrPasswordHashBusy`, `writePasswordBusy`.
- **Evidence:** FR-2.2, ADR 046, ADR 100 (owner, 2026-10-24). Code: compliant since #161. Over-1024-byte passwords are rejected before hashing with 422 `password_too_long` (REQ-AUTH-023), on own password change, Admin account save, and registration; tests `TestReqAuth023_*`, `TestArgon2id*`, `TestBcryptMigrationOnLogin`, `TestUpgradePasswordHashKeepsConcurrentChange`, `TestLoginBusyWithoutCountingAsFailure`.
- **Verification:** A 1025-byte password gets a 4xx validation error; 73 and 1024 bytes are accepted and hashed in full; a new stored value starts with `$argon2id$v=19$m=65536,t=3,p=1$`; a bcrypt account still logs in and afterwards has an Argon2id hash.

**SC-006 - Trustworthy client address behind the proxy**
- **Status:** ACTIVE
- **Rule:** With `TRUST_PROXY_HEADERS=true`, the client address MUST come from `X-Real-IP`, which nginx overwrites with `$remote_addr`. The leftmost `X-Forwarded-For` entry MUST NOT be used, because nginx appends to a client-supplied value. Without trusted proxy mode, `RemoteAddr` is used.
- **Enforcement:** `clientAddress` in `internal/httpapi/v1/security.go`; `deploy/nginx/raid-website.conf`.
- **Evidence:** FR-2.3, FR-7.6, ADR 046. Code: compliant since PR #44 (issue #59): `X-Real-IP` first, then the last `X-Forwarded-For` entry. Threats: T-001.
- **Verification:** Six failed logins with six different forged `X-Forwarded-For` values from one connection are blocked at the sixth.

**SC-007 - WebAuthn relying party from configuration**
- **Status:** ACTIVE
- **Rule:** In production, startup MUST fail without a valid HTTPS `PUBLIC_ORIGIN`, and the WebAuthn RP ID and origin MUST come only from it, never from `Host`, `X-Forwarded-Host`, or `X-Forwarded-Proto`.
- **Enforcement:** `internal/config`; `webAuthnRelyingParty`.
- **Evidence:** ADR 046, `deploy/README.md`. Code: compliant since #71 — `config.Load` stops production without an HTTPS `PUBLIC_ORIGIN` (origin only: no path, query, or user info), and `webAuthnRelyingParty` reads only it; forwarded headers are never read, and only development without `PUBLIC_ORIGIN` falls back to the request's own host. Tests `TestProductionPublicOrigin`, `TestReqAuth022_*`. Threats: T-008.
- **Verification:** `ENV=production` without `PUBLIC_ORIGIN` stops startup; a forged `X-Forwarded-Host` does not change the WebAuthn origin.

**SC-008 - Login throttling**
- **Status:** ACTIVE
- **Rule:** After 5 failed logins for the same normalised username and client address within 15 minutes, further attempts MUST be rejected without checking the password. Failures MUST be recorded in the database. The limit MUST hold for parallel requests: the attempt MUST be stored as failed in the same transaction that checks the limits, before the password is hashed. Each stored attempt MUST end in exactly one state at the credential decision: a wrong password, unknown username, or inactive account keeps it failed; correct credentials mark it successful at once, before MFA or session creation, so a later error never leaves it counted as a guess; when no password was checked (no free hashing slot, request ended, or a database error before the check) it MUST be deleted. Marking and deleting MUST run even when the request was cancelled, but for at most 5 seconds; a failure there fails closed (the attempt keeps counting as failed) and MUST be logged (ADR 103). This relies on the single database connection (DR-9).
- **Enforcement:** `auth.StartLogin`, `auth.loginAttempt`, `auth.Repo.ReserveLoginAttempt`, `login_attempts` table.
- **Evidence:** FR-2.3. Code: compliant (SC-006 fixed in PR #44; the parallel-request gap found in the review of 2026-10-25 is closed by ADR 103, tests `TestLoginLimitHoldsForParallelAttempts`, `TestParallelSuccessesAndFailuresCountOnlyWrongPasswords`, `TestCorrectPasswordIsNotAGuessWhenSessionFails`, `TestCancelledWaitReleasesTheReservation`). Threats: T-001, T-002.
- **Verification:** The sixth attempt in the window returns the rate-limit error even with the correct password; of 12 parallel wrong logins for one username exactly 5 reach the password check.

**SC-009 - No username probing**
- **Status:** ACTIVE
- **Rule:** Login for an unknown username, an account without a password, and a wrong password MUST return the same error and MUST take comparable time (run a password comparison against a fixed dummy hash with the current Argon2id parameters when no hash exists).
- **Enforcement:** `auth.StartLogin`.
- **Evidence:** V6. Code: compliant since #61 — unknown usernames and accounts without a password compare against `dummyPasswordHash` (Argon2id with the current parameters, ADR 100) and return the same `invalid_credentials`; inactive accounts run the normal comparison first. Threats: T-003, T-004.
- **Verification:** Median response time for an unknown and a known username differ by less than the cost of one password hash; `TestReqAuth012_*` checks that each path runs exactly one password comparison. Accounts with a legacy bcrypt hash are slightly faster until their next login (ADR 100).

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
- **Evidence:** FR-3.9, V6, OD-002 (owner, 2026-10-22). Code: compliant since #62 — `mfa_login_challenges.failed_attempts` (migration 077) counts wrong codes; the fifth deletes the challenge and answers `401 mfa_challenge_invalid`, and the login page returns to the password step. Wrong WebAuthn assertions do not count. Threats: T-005.
- **Verification:** After 5 wrong codes, the correct code for that challenge is rejected and a new password login is needed.

**SC-012 - TOTP codes used once**
- **Status:** ACTIVE
- **Rule:** TOTP MUST use 6 digits, 30-second steps, and at most ±1 step of drift, and a code that was already accepted for an account MUST NOT be accepted again within its validity window.
- **Enforcement:** `auth.acceptTOTP` and `auth.Repo.ClaimTOTPStep`; `player_mfa_settings.totp_last_step` stores the last accepted time step.
- **Evidence:** FR-3.2, V6. Code: compliant since #63 — login and TOTP enable claim the code's time step with one conditional update (migration 078), so a code for that step or an earlier one is rejected and of two simultaneous requests exactly one succeeds. Threats: T-006.
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
- **Evidence:** V11, V14, OD-003 (owner, 2026-10-22). Code: encryption and transactional plaintext migration built in #64. The server requires a base64-encoded 32-byte environment key whenever stored TOTP exists; new setups cannot store plaintext without a key. Previous-key rotation is built in #65: startup accepts configured previous keys and re-encrypts every old-key value under the current key in the same migration transaction. Unknown keys stop startup. Threats: T-011.
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
- **Evidence:** FR-2.13, FR-2.14, FR-5.4, FR-30.2, OD-001 (owner, 2026-10-22). Code: compliant since #66 — `ChangeOwnPassword` deletes the user's other sessions and keeps the current one, and `SetCredentials` deletes all sessions of the player when password, login name, or role changes (unchanged saves keep them), each in the same transaction as the change. Threats: T-010.
- **Verification:** After a password change, a second session cookie returns 401 and the current one still works.

**SC-018 - Session lifetime**
- **Status:** ACTIVE
- **Rule:** A session MUST end 30 days after login at the latest, and MUST end earlier when it has not been used for 7 days. Each authenticated request resets the idle clock; the last-use time MUST be written at most once per hour, so that ordinary reads do not write to the database.
- **Enforcement:** `auth` session validation; a last-used timestamp on `sessions`.
- **Evidence:** FR-2.4, FR-2.12, V7, OD-008 (owner, 2026-10-22). Code: compliant since #67: authentication rejects seven-day idle sessions and the fixed 30-day expiry; a conditional update writes `last_seen_at` at most hourly, including concurrent requests. Touch failures are logged without failing the read. Threats: T-010.
- **Verification:** A session unused for 7 days and 1 minute returns 401; a session used daily stays valid until day 30.

**SC-019 - CSRF protection**
- **Status:** ACTIVE
- **Rule:** Every state-changing request (not GET, HEAD, OPTIONS) MUST carry an `X-CSRF-Token` header equal to the CSRF cookie, compared in constant time, whether or not it carries a session cookie, `Origin`, or `Sec-Fetch-Site`. This includes login, MFA verification, and registration. The token MUST be 32 bytes from `crypto/rand`.
- **Enforcement:** `csrfProtection` middleware; `GET /api/v1/auth/csrf`; `web/src/api.ts`.
- **Evidence:** FR-2.8, OD-009 (owner, 2026-10-22). Code: compliant since #68 — `csrfProtection` checks every non-GET/HEAD/OPTIONS request, with or without a session cookie, `Origin`, or `Sec-Fetch-Site` (`TestReqAuth019_*`). `web/src/api.ts` sends the token on every non-GET request. Threats: T-007.
- **Verification:** A POST with the session cookie, no CSRF header, and no `Origin` returns 403 `csrf_required`.

### Input, output, and API (V1, V2, V4)

**SC-020 - Strict request decoding**
- **Status:** ACTIVE
- **Rule:** JSON bodies MUST be decoded with `DisallowUnknownFields` and a 1 MiB limit; path IDs MUST be positive integers; enums, roles, classes, specs, statuses, and recruitment slot values MUST be validated on the server.
- **Enforcement:** `DecodeJSON`, `pathID`, `internal/validation`, domain services.
- **Evidence:** NFR-4.1, FR-23.4. Code: compliant; recruitment slot values are checked on the server against the editor's list, and a bad value answers 422 `invalid_slot` (#93). Stored values that the list no longer accepts are logged at startup, never changed. Threats: T-018, T-027.
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
- **Evidence:** FR-7.5, ADR 053. Code: compliant since #113: known domain errors use stable codes and fixed German messages; unexpected account-save failures are logged and return generic `500 internal`. No response writer uses `err.Error()`.
- **Verification:** Forced repository failure returns `internal` with a generic message.

### Web frontend and transport (V3, V12)

**SC-024 - Security headers**
- **Status:** ACTIVE
- **Rule:** Every response MUST carry `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`, `X-Frame-Options: DENY`, the restrictive `Permissions-Policy`, and a CSP with `default-src 'self'`, `script-src 'self'`, `object-src 'none'`, `frame-ancestors 'none'`, `base-uri 'self'`, `form-action 'self'`, `connect-src 'self'`. `img-src` MUST be `'self' data: https://wow.zamimg.com`: class icons are local (DQ-2), and the host stays only for the item icons of the loot catalog (ADR 104). HSTS (`max-age=31536000; includeSubDomains`) MUST be sent in production.
- **Enforcement:** `securityHeaders` middleware.
- **Evidence:** ADR 046, `DESIGN.md`. ADR 104. Code: compliant; class icons are served from `/class-icons/`, `img-src` allows `https://wow.zamimg.com` for item icons only; `style-src` allows `'unsafe-inline'`. Threats: T-009, T-024, T-027.
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
- **Evidence:** NFR-4.2, ARCHITECTURE "Server lifecycle", OD-005 (owner, 2026-10-22). Code: compliant — `cmd/server` builds an `http.Server` with these limits and shuts down on SIGTERM/SIGINT. Threats: T-017.
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
- **Evidence:** FR-7.6, AQ-7, OD-004 (owner, 2026-10-22). Code: compliant since #79: the registration service shares a mutex-protected in-memory ten-request budget across visitor searches and registration requests; cached answers bypass it, authenticated callers are exempt, and expired address counters are removed. Threats: T-016, T-035.
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
- **Rule:** Anonymisation (FR-30.2–30.4) MUST run in one transaction and MUST remove every credential row of the player: password hash, sessions, MFA settings, recovery codes, WebAuthn credentials (SC-017), and push subscriptions (SC-053).
- **Enforcement:** `accountmerge.Service.Anonymise`, `Repo.AnonymiseAccount`; `audit.Repo.List` (display).
- **Evidence:** FR-30, AQ-6. Code: compliant since #85 — one transaction deletes sessions, WebAuthn and MFA rows, push subscriptions, the player's login attempts, and clears the Rising-Gods profile fields on the player's characters and registration requests (unused codes of those requests are revoked); the login name and password hash are cleared and the role becomes GUEST. It answers `409 last_admin` (SC-047) and `409 already_anonymised`. Tests `TestReqPrivacy011_*`, `TestReqPrivacy011_AC05` in `internal/accountmerge`.
- **Verification:** After anonymisation no row links the player to a login, secret, or external profile; old sessions return 401.

**SC-033 - Secrets in configuration**
- **Status:** ACTIVE
- **Rule:** `ADMIN_PASSWORD`, `RISING_GODS_PROXY_TOKEN`, and the privacy contact values MUST come only from `/etc/raid-website.env` (readable by root and the service only) or Cloudflare secrets, never from the repository. Production MUST NOT start with default admin credentials.
- **Enforcement:** `internal/config`, `deploy/`.
- **Evidence:** NFR-1.2, FR-30.1, ADR 020, ADR 048. Code: compliant.
- **Verification:** A secret scan of the repository finds none; production with default credentials fails startup.

**SC-034 - Off-server backups**
- **Status:** ACTIVE
- **Rule:** The daily off-server copy (newest verified snapshot plus `/etc/raid-website.env`) MUST be encrypted with GnuPG symmetric AES-256 before it leaves the host. The passphrase is in `/etc/raid-website-backup/passphrase` (root, 0400), and the owner keeps a second copy off the host; it MUST NOT be committed. The server MUST reach the Storage Box only through its own sub-account (SSH key, pinned host key, Hetzner network only, one folder). It MUST only add files: upload under a temporary name, rename without replacing, compare against a download, never delete. The server MUST NOT hold a Hetzner API token, so it cannot change the Storage Box snapshots.
- **Enforcement:** `deploy/bin/raid-website-backup` (`--offsite`, `--restore-test`); `raid-website-backup-offsite.timer` (daily), `raid-website-backup-restore-test.timer` (monthly); Storage Box snapshot plan (daily, 10 kept).
- **Evidence:** NFR-2.3, NFR-2.5, ADR 070, ADR 099, OD-007 (owner, 2026-10-24). Code and host: compliant since #109. The first copy was uploaded on 2026-10-24 and restored on the host and on the owner's PC; the site started with the restored data. Threats: T-011, T-030.
- **Verification:** `systemctl start raid-website-backup-offsite.service` adds one `.tar.gpg` and its `.sha256`, and a second run adds nothing. `systemctl start raid-website-backup-restore-test.service` succeeds. A wrong target makes the upload unit fail. Decrypting with the owner's passphrase on another machine gives a database that passes `PRAGMA integrity_check`.

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
- **Evidence:** ADR 071, ADR 072, DR-11, OD-006. Code: lock files and permissions compliant; every `uses:` in `.github/workflows/` is pinned to a full commit SHA with the tag as a comment, and `.github/dependabot.yml` proposes weekly updates for GitHub Actions, Go modules, and npm (`/web`). Test: `internal/ciconfig` (`TestReqPlatform014_AC01`, `TestReqPlatform014_AC02`). Threats: T-012, T-014, T-028.
- **Verification:** Workflow review; `npm ci` and `go build` fail on a lock-file mismatch.

**SC-037 - Deployment scripts under review**
- **Status:** ACTIVE
- **Rule:** Every script that `deploy/README.md` installs (`raid-website-publish`, `-update`, `-backup`, `-configure-ufw`) MUST be committed under `deploy/bin/` and reviewed like code. `.gitignore` MUST NOT exclude `deploy/bin/`. The installed copies MUST match the committed ones.
- **Enforcement:** `.gitignore`, `deploy/bin/`, `deploy/README.md`.
- **Evidence:** ADR 071, ADR 072. Code: compliant since #105 — `.gitignore` ignores only the root `/bin/`, the four scripts are committed under `deploy/bin/` (`TestReqPlatform013_AC01` in `internal/ciconfig`), CI runs `shellcheck` on them, and on 2026-10-24 the installed copies in `/usr/local/sbin` were identical to the committed ones. Threats: T-013, T-029.
- **Verification:** `git ls-files deploy/bin` lists all four scripts; a diff of the installed scripts against the repository is empty.

**SC-038 - Short-lived security records are deleted**
- **Status:** ACTIVE
- **Rule:** Login attempts, failed or successful, MUST be deleted once they are older than the longest login-limit window that uses them (SC-008, SC-043); a successful login MUST NOT be kept as an activity history. Expired sessions and expired or used MFA challenges MUST be deleted. Deletion runs during normal requests or at startup, not as a background job.
- **Enforcement:** `auth.Repo.PruneSecurityRecords`, called at startup and before login.
- **Evidence:** FR-30.6, ARCHITECTURE "No background jobs". Code: compliant since #74: startup and login delete attempts older than 15 minutes, expired sessions, and used or expired MFA and WebAuthn challenges in one transaction. Cleanup failures are logged and do not change the login result. Threats: T-022.
- **Verification:** After the window, no login attempt, expired session, or old challenge row remains.

**SC-039 - Protected main branch and CI gates**
- **Status:** ACTIVE
- **Rule:** `main` MUST accept changes only through pull requests whose required CI checks pass; direct pushes, force pushes, and deletion MUST be blocked. No approving review is required (one maintainer). Required checks MUST include the Go and frontend gates and a vulnerability scan: `govulncheck` for Go and `npm audit --omit=dev` for the frontend; a known vulnerability blocks the merge until it is fixed.
- **Enforcement:** GitHub ruleset on `main`; `.github/workflows/ci.yml`.
- **Evidence:** ADR 071, ADR 072, NFR-5.1, OD-011 (owner, 2026-10-23). Code: ruleset "Protect" active since 2026-10-23 (pull requests only, squash, required checks `quality` and `e2e`, no bypass); `quality` runs `govulncheck` and `npm audit --omit=dev --audit-level=high` (#102). Threats: T-012, T-028.
- **Verification:** A direct push to `main` is rejected; a pull request with a failing test or a known vulnerable dependency cannot merge.

**SC-040 - MFA required for Admins**
- **Status:** ACTIVE
- **Rule:** An ADMIN account without enabled TOTP or a security key MUST be sent to MFA setup after login, and every other Admin-only request MUST be rejected with HTTP 403 `mfa_setup_required` until MFA is enabled. This includes the bootstrap Admin and a player who is newly given the ADMIN role. Responses shared with other roles MUST NOT show such an Admin the Admin-only fields of other players (login name, role, account timestamps); the Admin sees them like a RAIDLEADER (ADR 103). Other roles stay optional (FR-3.1).
- **Enforcement:** `auth` login flow; `RequireRole(ADMIN)` route guard.
- **Evidence:** FR-3.10, OD-012 (owner, 2026-10-23). Code: compliant after #69 — the session middleware records whether an ADMIN has TOTP or a security key, `RequireRole(ADMIN)` answers 403 `mfa_setup_required` without one, and the web client sends such an Admin to the account tab. `GET /api/v1/players/{id}` hides other players' account fields from it (`accountViewer`, test `TestReqAuth020_AC07_*`). Threats: T-020.
- **Verification:** An Admin without MFA gets 403 `mfa_setup_required` on `GET /api/v1/audit-log` and succeeds after enabling TOTP.

**SC-041 - Security event logging**
- **Status:** ACTIVE
- **Rule:** The server MUST write one structured journal line for each failed login, login or lookup rate-limit rejection, wrong MFA code, invalidated MFA challenge, CSRF rejection, password change or reset, Admin change of a player's credentials or role, and MFA enable or disable. Each line MUST contain time, event name, player ID or normalised username, client address (SC-006), and request ID, and MUST NOT contain passwords, codes, or tokens (SC-030). The journal MUST keep these lines for 30 days.
- **Enforcement:** `auth`, `csrfProtection`, rate limiters; journald retention in `deploy/`.
- **Evidence:** FR-26.4 (logins stay out of the audit log), OD-013 (owner, 2026-10-23). Code: partly compliant since #73 — `internal/httpapi/v1/securityevents.go` writes one slog JSON line to stderr (journal) for `login_failed`, `login_blocked`, `mfa_failed`, `mfa_challenge_invalid`, `csrf_rejected`, `password_changed`, `account_changed`, `mfa_enabled`, and `mfa_disabled`, with client address and request ID; retention is in `deploy/README.md`; test `TestReqAuth024_SecurityEventsWithoutSecrets`. Still missing: the lookup rate-limit rejection (SC-028 not built) and password reset events (SC-042 not built). Wrong MFA codes and invalidated challenges carry the `player_id` of the challenge (none for an unknown or expired one). Threats: T-021.
- **Verification:** Five wrong logins produce five journal lines without the password; journald keeps them 30 days.

**SC-042 - Password reset with an in-game code**
- **Status:** ACTIVE
- **Rule:** A password reset MUST use a single-use reset code that an Admin creates for a player and sends in game to that player's main character, with the same randomness, hashing, and revocation as registration codes (SC-013) and a lifetime of 72 hours. Redeeming it MUST set a new password (SC-005), MUST remove the player's TOTP settings, recovery codes, and security keys, and MUST end all sessions of the player (SC-017), in one audited transaction. An Admin MUST NOT set another player's password directly for a reset. Creating a reset code MUST revoke any earlier unused reset code of the same player. Failed redemptions of reset and registration codes MUST count against the per-address limit of SC-043. Creating and redeeming a reset code MUST be logged as security events (SC-041). The in-game message is the only notification channel; a player who did not ask for the reset notices because their old password stops working (accepted: the site stores no e-mail). A reset of an ADMIN is followed by SC-040.
- **Enforcement:** reset-code workflow in `registration`, on the registration-code infrastructure (owner 2026-10-23, backlog #48); `auth`.
- **Evidence:** FR-5.8, OD-014, OD-018 (owner, 2026-10-23). Code: Admin code creation, 72-hour expiry, hash-only storage, replacement revocation and the prohibition on directly setting another player's password are built in #81. Redemption is built in #82: one transaction changes the password, consumes the code, removes MFA and all sessions, revokes push delivery and records counts without secrets. Reset and registration redemptions share the per-address failed-attempt budget. The operator command is tracked in #83. Threats: T-026.
- **Verification:** A redeemed code works once; afterwards old sessions return 401, MFA is off, and the new password logs in; a second code revokes the first; the 31st failed redemption from one address in 15 minutes is rejected; a code older than 72 hours is rejected.

**SC-043 - Login limit per client address**
- **Status:** ACTIVE
- **Rule:** Besides SC-008, one client address (SC-006) MUST NOT make more than 30 failed login attempts, for any usernames, in 15 minutes; further attempts are rejected with the rate-limit error before any password check. Successful logins do not count (owner 2026-10-23, ADR 093). There is no separate per-account limit across addresses (T-002 accepted).
- **Enforcement:** `auth.StartLogin`, `login_attempts`.
- **Evidence:** FR-2.3, OD-010 (owner, 2026-10-23), ADR 093, ADR 103. Code: compliant since #60 — `auth.StartLogin` counts failed attempts per address (`CountFailedLoginAttemptsFromAddress`, index from migration 076) before the per-username check and before any password hash; since ADR 103 the count and the stored attempt share one transaction, so parallel requests are counted (`TestAddressLimitHoldsForParallelAttempts`). Threats: T-002, T-003.
- **Verification:** After 30 failed logins from one address within 15 minutes, the next attempt is rejected without a password comparison (`TestReqAuth011_*`).

**SC-044 - Incident plan and secret rotation**
- **Status:** ACTIVE
- **Rule:** `deploy/README.md` MUST contain an incident plan with these steps: end all sessions (the documented database command that deletes every session row), rotate `ADMIN_PASSWORD`, the Worker token, the Cloudflare API token, the TOTP key, and the VAPID key (SC-054), pause automatic deploys (stop `raid-website-update.timer`), and restore from a verified snapshot. There is no in-app "log everyone out" action. The owner is the only person who acts (T-040 accepted).
- **Enforcement:** `deploy/README.md`.
- **Evidence:** OD-015 (owner, 2026-10-23). Code: plan written in `deploy/README.md` ("Incident plan and secret rotation", #108): detection, containment, rotation, restore, review. The TOTP key step says SC-014 is not built, and the Admin password step notes that the env variable does not change an existing account (FR-2.10) and refers to `cmd/reset-code` (SC-051, not built). Owner dry run still pending. Threats: T-040, T-041.
- **Verification:** Each step was carried out once on a test setup and old session cookies returned 401 afterwards.

**SC-045 - Patched Go toolchain**
- **Status:** ACTIVE
- **Rule:** `go.mod` MUST name a `toolchain` at least as new as the newest Go patch release that fixes every reachable `govulncheck` finding (today `go1.26.10`), so that CI and the production build use it even when the host's Go is older. The vulnerability scan of SC-039 MUST run with that toolchain.
- **Enforcement:** `go.mod`; `.github/workflows/ci.yml`; the host update script (SC-037).
- **Evidence:** `THREAT_MODEL_CODE.md` CT-001: 29 reachable standard-library vulnerabilities with `go1.26.0`, all fixed in `go1.26.10`; `go.mod` now names `toolchain go1.26.10`, CI checks that its Go is at least that version, and the update service sets `GOTOOLCHAIN=auto` (#102). Code: compliant in the repository; production compliant once the updated unit is installed (`deploy/README.md`). Threats: T-046.
- **Verification:** `govulncheck ./...` with the toolchain from `go.mod` reports no reachable vulnerability; `go version -m` of the production binary shows that toolchain.

**SC-046 - No account hints before a valid code**
- **Status:** ACTIVE
- **Rule:** Public endpoints that take a registration or reset code MUST validate the code before they reveal anything about accounts; with an invalid code they MUST answer `registration_code_invalid` whatever the username. The username check MUST run inside the code transaction, and a duplicate username there MUST map to `409 username_taken`, not to a server error.
- **Enforcement:** `registration.RedeemNewAccount`; the reset-code flow (SC-042).
- **Evidence:** `THREAT_MODEL_CODE.md` CT-003. Code: compliant for `registration.RedeemNewAccount` since #75 — the username check runs inside the code transaction after the code is validated, and a duplicate answers `409 username_taken` (`TestReqAccount010_*`); the reset-code flow still has to follow (SC-042). Threats: T-042.
- **Verification:** With an invalid code, a taken and a free username get the same answer.

**SC-047 - The last Admin stays an Admin**
- **Status:** ACTIVE
- **Rule:** A role change, deactivation, merge, or anonymisation that would leave no active ADMIN MUST be rejected with `409 last_admin`.
- **Enforcement:** `auth.SetCredentials`, `accountmerge`, the anonymise operation.
- **Evidence:** FR-5.9; `THREAT_MODEL_CODE.md` CT-006. Code: compliant for role change and merge since #80 — `auth.SetCredentials` and `accountmerge.Merge` count active Admins inside the write transaction and answer `409 last_admin` (`player.ErrLastAdmin`); a merge into an active Admin target passes. No code path deactivates a player yet; the anonymise operation (REQ-PRIVACY-011) calls the same check. Tests `TestReqAccount015_AC01` to `AC03`, frontend message test. Threats: T-044.
- **Verification:** Demoting the only Admin returns 409 and changes nothing.

**SC-048 - Cloned security keys are rejected**
- **Status:** ACTIVE
- **Rule:** When the WebAuthn library reports a clone warning (the signature counter did not increase), the login MUST be rejected, the event logged (SC-041), and the counter left unchanged.
- **Enforcement:** `auth.FinishWebAuthnMFA`.
- **Evidence:** `THREAT_MODEL_CODE.md` CT-008. Code: compliant since #70 — `auth.FinishWebAuthnMFA` rejects a login whose credential carries the clone warning with `ErrInvalidMFA` (`401 invalid_mfa_code`) before the counter is stored; the `mfa_failed` journal line carries `reason=cloned_key`. Authenticators that always report counter 0 are not flagged by the library. Threats: T-045.
- **Verification:** An assertion with a lower counter than stored fails.

**SC-049 - The Admin confirms a match by character name**
- **Status:** ACTIVE
- **Rule:** When a registration request has no character linked to its Rising-Gods profile but a player without a login owns a character with the requested name, the Admin MUST see that player before creating the code and MUST choose either "attach to this player" or "new player". The choice MUST be stored with the code. Redeeming MUST follow the stored choice and MUST NOT match by name on its own.
- **Enforcement:** `registration` code creation and `RedeemNewAccount`; the admin request list.
- **Evidence:** FR-4.5, OD-016 (owner, 2026-10-23). Code: compliant since #77 — `registration.CreateCode` looks for a player without a login that owns a character with the requested name (no character linked to the profile), answers `422 match_choice_required` without a choice and `422 match_choice_invalid` for a wrong one, stores `match_choice` and `match_player_id` on the code (migration 082) and records them in the audit entry; `RedeemNewAccount` attaches only the stored player and otherwise creates a new player with a character of its own, never matching by name. Codes made before migration 082 have no choice and redeem as a new player. The admin request list shows the match (`name_match`). Tests `TestReqAccount012_*`. Threats: T-043.
- **Verification:** A request whose name matches an imported player cannot get a code until the Admin chooses; with "new player" the redeem creates a new player even though the name matches.

**SC-050 - Only the profile fields the site uses are stored**
- **Status:** ACTIVE
- **Rule:** From a Rising-Gods profile the system MUST store only character name, class, specs, and gear score. The raw response MUST NOT be stored; existing raw copies on registration requests and characters MUST be removed by a migration.
- **Enforcement:** `registration` and `characters` repositories; a database migration that drops or empties `raw_profile_json` and `external_profile_json`.
- **Evidence:** FR-30.6, OD-017 (owner, 2026-10-23); `THREAT_MODEL_CODE.md` CT-030. Code: compliant since #78: repositories discard raw profiles and unused demographic fields; migration 083 clears existing copies. Profile identifiers and URLs remain for linking. Threats: T-047.
- **Verification:** After registration and after the migration, no row holds a raw profile; the four fields are still shown.

**SC-051 - Server-side reset code for a locked-out Admin**
- **Status:** ACTIVE
- **Rule:** A command `cmd/reset-code`, run on the server as the application user, MUST create a reset code (SC-042) for a named player and print it once. It MUST open the database directly, MUST NOT be reachable over HTTP, MUST write an audit entry with the actor "server operator", and MUST be logged as a security event (SC-041). It MUST NOT set a password or remove MFA itself; that happens only when the code is redeemed.
- **Enforcement:** `cmd/reset-code` (new) using the `registration` reset-code service.
- **Evidence:** FR-3.11, OD-019 (owner, 2026-10-23). Code: built in #83: `cmd/reset-code` uses the shared reset service, audits as "server operator", logs a security event without the code and prints it once. Threats: T-049.
- **Verification:** Running the command for the only Admin prints a code that works once within 72 hours; the audit log shows the operator entry; no HTTP route reaches the command.

**SC-052 - Service worker without cache; push shows fixed texts only**
- **Status:** ACTIVE
- **Rule:** The service worker (`/sw.js`) MUST NOT register a `fetch` handler, MUST NOT use Cache Storage, and MUST NOT load other scripts, so no page or API response is kept on the device and every request reaches the server with its normal session and CSRF checks. Its `push` handler MUST read only the `kind` of a message and MUST show the fixed German text for that kind (FR-32.5), or a fixed fallback text for an unknown or unreadable message; it MUST NOT display any other field of the message. Its `notificationclick` handler MUST open only same-origin pages. Adding caching, other message content, or other handlers needs a new owner decision.
- **Enforcement:** `web/public/sw.js`; `web/src/components/PWAInstall.tsx` (registration only).
- **Evidence:** FR-28.5, FR-32.5, ADR 101, ADR 102 (owner, 2026-10-24; amended for push). Code: compliant; tests `the service worker caches nothing and handles no requests` and `a push shows only a fixed German text for its kind` in `web/tests/frontend.test.cjs`.
- **Verification:** After a visit, `caches.keys()` is empty and `sw.js` contains no `fetch` listener; a push whose payload carries extra fields shows only the fixed text.

**SC-053 - Push subscriptions and delivery**
- **Status:** ACTIVE
- **Rule:** A subscription endpoint MUST be an `https` URL on port 443 without user info whose host is one of `fcm.googleapis.com`, `updates.push.services.mozilla.com`, a subdomain of `push.apple.com`, or a subdomain of `notify.windows.com`; `p256dh` MUST be a valid uncompressed P-256 point and `auth` 16 bytes. Anything else is rejected with `400 invalid_push_subscription` and nothing is stored. The sender MUST NOT follow redirects, MUST time out after 10 s per message and 15 s per fan-out, and MUST read at most 4 KiB of an answer. A message payload MUST be the JSON `{"kind": …}` only, encrypted per RFC 8291 (aes128gcm) with a fresh key and salt per message. Endpoints and keys MUST NOT appear in API responses, logs, or audit snapshots; the audit entry keeps the push-service host only. Logs name the subscription ID and HTTP status, never the endpoint (SC-030). A subscription MUST belong to one player on one device. The device is identified by the `raid_device` cookie: 32 random bytes from `crypto/rand`, `HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` in production, 400 days; the database stores only its SHA-256 (FR-32.3, ADR 103). Every session start (login, MFA, security key, registration) MUST first delete the subscriptions of other players on that device; if that fails, no session cookie is issued. Every session start MUST renew the device cookie's lifetime, and a browser without one MUST get a new one. Logout MUST delete the actor's subscriptions on that device. `POST /api/v1/push/subscriptions/sync` (endpoint in the body, never in a URL) reports whether the endpoint is subscribed for the actor, deletes it when it belongs to another player, and attaches the actor's own subscription to this device. The server cannot tell which subscription a browser without its device cookie holds, so the web client MUST check in after every sign-in and before it shows the player as signed in, and MUST drop the browser's subscription when the server does not confirm it or the check-in fails or exceeds 5 seconds (fail closed; signing in never depends on push). The 5 seconds are one deadline for the whole check-in, queue wait included; at the deadline the request is aborted and a late answer changes nothing, and dropping the subscription is started without waiting for it. No push step waits longer than 10 seconds for the one before it; a change that starts this way cancels the stalled one, which then makes no further change. A timed-out check-in drops only the subscription it examined, and only if the browser still holds it. Subscriptions without a device MUST get no messages and MUST be deleted 30 days after creation if still unclaimed (at startup, no background job). The web client MUST NOT re-subscribe on its own and MUST run its push changes one at a time.
- **Enforcement:** `internal/push` (`SubscriptionInput.validate`, `HTTPSender`, `auditView`, `Service.Sync`, `ClaimDevice`, `ForgetDevice`, `RevokeForPlayer`); `internal/httpapi/v1/push.go` (`ensureDevice`), `auth.go` (`startSession`, `logout`); `auth.ChangeOwnPassword`, `auth.SetCredentials`; migration 081; `web/src/pushNotifications.ts` (`runExclusive`, `checkInPush`, `checkInSubscription`); `push.Service.DeleteUnclaimed` at startup in `cmd/server`.
- **Evidence:** FR-32, ADR 102 (owner, 2026-10-24). Code: compliant; tests `TestReqPush001_*` (RFC 8291 test vector), `TestReqPush003_EndpointAllowlist`, `TestReqPush005_SubscribeValidatesAndAudits`, `TestReqPush011_OptInSurvivesSignInOnTheSameDevice`, `TestReqPush012_AnotherPlayerOnTheDeviceEndsTheOptIn`, `TestReqPush014_SyncReportsAndClears`, `TestReqPush015_SignOutAndAccountChanges`, `TestReqPush016_SignInWithoutDeviceCookie`, `TestReqPush017_UnclaimedRowsGetNothingAndExpire`; frontend `the push check-in keeps only a subscription the server confirms and fails closed`.
- **Verification:** A subscription for `https://127.0.0.1/` or `http://fcm.googleapis.com/` returns 400 without an outbound request; the audit log and the journal contain no endpoint path.

**SC-054 - VAPID key**
- **Status:** ACTIVE
- **Rule:** The VAPID private key (`VAPID_PRIVATE_KEY`, base64url P-256 scalar) and its contact (`VAPID_SUBJECT`, `mailto:` or `https:`) MUST come only from `/etc/raid-website.env` (SC-033), are set together or not at all, and MUST NOT be committed, logged, or sent to the client; only the public key is served (`GET /api/v1/push/config`). An invalid key MUST stop startup; a missing key MUST switch push off (FR-32.6). The key is part of the daily encrypted off-server copy of the env file (SC-034) and MUST be rotated with the other secrets in the incident plan (SC-044); rotation invalidates every subscription. The JWT MUST be ES256, `aud` the push-service origin, `exp` at most 12 hours ahead (RFC 8292). New keys come from `go run ./cmd/vapid-key`.
- **Enforcement:** `internal/config`, `cmd/server` (`push.ParseVAPIDKey`), `internal/push/webpush.go`; `deploy/README.md`.
- **Evidence:** FR-32.6, ADR 102 (owner, 2026-10-24). Code: compliant; tests `TestVAPIDConfiguration`, `TestReqPush002_*`.
- **Verification:** A key without a subject stops startup; `GET /api/v1/push/config` returns the public key and nothing else; the repository secret scan finds no key.

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

- **OD-007** Off-server backup to a Hetzner Storage Box. The passphrase is kept on the server and by the owner. Reading a copy is acceptable, overwriting one is not: uploads only add files, and Storage Box snapshots are outside the server's reach → SC-034, ADR 099 (owner, 2026-10-24).

Decided by the owner on 2026-10-23 (threat model):

- **OD-010** Per-address login limit of 30 failed attempts in 15 minutes (successful logins do not count, ADR 093); no per-account limit → SC-043.
- **OD-011** `main` only through pull requests with passing CI and a blocking vulnerability scan; no second approver → SC-039.
- **OD-012** MFA required for Admins → SC-040.
- **OD-013** Security events to the server journal, kept 30 days → SC-041.
- **OD-014** Password reset with an in-game code that also removes MFA → SC-042.
- **OD-015** Written incident plan, no in-app button; the owner stays the only key holder → SC-044.
- **OD-016** A match by character name is attached only after the Admin confirms it when creating the code → SC-049, FR-4.5.
- **OD-017** Only name, class, specs, and gear score are kept from a Rising-Gods profile; raw copies are dropped → SC-050, FR-30.6.
- **OD-018** A password reset code lasts 72 hours → SC-042.
- **OD-019** A locked-out only Admin gets a reset code from a server command → SC-051, FR-3.11.
- **OD-020** OD-007 (off-server backup) is decided by 2026-11-30 (decided 2026-10-24).

Decided by the owner on 2026-10-24 (Web Push):

- **OD-021** Opt-in Web Push for a new poll, a new raid, and the own roster place; RFC 8291/8292 with the standard library, no new dependency; VAPID key in the env file and its off-server copy, push off without it → SC-052, SC-053, SC-054, FR-32, ADR 102.

Decided by the owner on 2026-10-25 (review fixes):

- **OD-022** Review fixes of 2026-10-25: login attempts are reserved before hashing and end in exactly one state at the credential decision; a 5-second, cancellable wait for a hashing slot with `503 server_busy`; push subscriptions belong to a player on a device, survive that player's next sign-in there, and never reach another player; an Admin without MFA does not see other players' account fields; a GUEST player attached by a registration code becomes MEMBER → SC-005, SC-008, SC-040, SC-043, SC-053, FR-4.5, FR-32.3, ADR 103.
