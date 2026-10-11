# ADR 046: Account Security Hardening

Accepted.

## Context

The site uses local accounts with bcrypt passwords and cookie-backed sessions. The owner wants optional MFA, authenticator-app TOTP, YubiKey support, a 12-character password minimum, and deployment behind nginx as a reverse proxy.

## Decision

- MFA is optional per account. Accounts without enabled MFA keep the existing username/password login flow.
- Authenticator-app MFA uses standard TOTP secrets and six-digit codes.
- YubiKey support is implemented as WebAuthn/FIDO2 security keys, not Yubico OTP.
- The project may add `github.com/go-webauthn/webauthn` for WebAuthn challenge and assertion handling.
- Passwords for new or changed credentials must be at least 12 characters after trimming whitespace.
- Cookie sessions remain `HttpOnly` and `SameSite=Lax`; production deployments behind nginx must set secure cookies.
- Reverse-proxy headers are trusted only when explicitly enabled by configuration. Trusted proxy mode may use `X-Forwarded-For`, `X-Real-IP`, and `X-Forwarded-Proto` for rate limiting and origin-aware security behavior.
- Public origin configuration is required for WebAuthn in deployments where nginx terminates TLS.
- Unsafe cookie-authenticated mutations require CSRF protection.
- Login attempts are rate-limited by normalized username and client address.

## Consequences

- Login may become a two-step flow: valid password first, then TOTP, recovery code, or WebAuthn assertion before the real session cookie is issued.
- Recovery codes are displayed once and stored only as hashes.
- Account-security UI is needed so users can manage their own optional MFA methods.
- nginx deployments must forward the original host, scheme, and client IP consistently.
