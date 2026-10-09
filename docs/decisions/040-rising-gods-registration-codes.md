# ADR 040: Rising-Gods-backed registration codes

Status: Accepted

## Decision

Player registration and character claiming use Rising-Gods profiler characters as the required
external character identity. A request can only be created from a Rising-Gods profile result; manual
character-name fallback is intentionally not supported because characters missing from the profiler
are treated as inactive for onboarding.

Admins generate single-use registration codes for pending requests and send the plaintext code
in-game to the selected character. The website stores only a hash of the code. Codes expire after
14 days and can be revoked while unused.

Redeeming a new-account code creates a `GUEST` player account, creates or links the selected
character, starts a session, and consumes the code. Redeeming a claim code links or transfers the
selected character to the authenticated player and writes an audit entry when ownership changes.

The first Rising-Gods talent build becomes the local character's main spec. The second build becomes
the local second spec when present; second spec may be empty.

## Consequences

Rising-Gods lookup is used for usability and eligibility, but ownership proof remains delivery of the
code to the in-game character. Existing raid snapshots are not rewritten by character claiming.
