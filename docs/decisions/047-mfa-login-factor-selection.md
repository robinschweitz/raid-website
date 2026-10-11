# ADR 047: MFA Login Factor Selection

Accepted.

## Context

ADR 046 added optional TOTP, recovery-code, and WebAuthn MFA. The login challenge originally only told the frontend that MFA was required, not which factors the account actually had. That made the login UI show a generic MFA code field even for accounts that only added WebAuthn.

## Decision

- The password login response for an MFA-enabled account must include the available MFA factors for that challenge.
- The frontend must only show MFA controls for factors the account actually configured.
- WebAuthn/FIDO2 security keys are phishing-resistant MFA and must be the default login option when present and supported by the browser.
- TOTP and recovery-code login remain available only when those factors exist for the account.

## Consequences

- WebAuthn-only users see a security-key login prompt, not a code input.
- Users with both WebAuthn and code-based MFA see WebAuthn first, with code login as an explicit fallback.
- The server remains the source of truth for factor availability; the client only renders the options it receives for the challenge.
