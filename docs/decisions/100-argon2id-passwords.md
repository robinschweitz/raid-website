# ADR 100: Passwords are hashed with Argon2id

Status: Accepted. Amends ADR 046 (password hashing) and REQ-AUTH-023 (72-byte limit).

## Context

ADR 046 hashed passwords with bcrypt at `DefaultCost`. Because bcrypt reads at most 72 bytes,
REQ-AUTH-023 rejected longer passwords (SC-005). On 2026-10-24 the owner chose to move to
Argon2id (PR #161). Argon2id is memory-hard and is the first choice of the OWASP Password
Storage Cheat Sheet and RFC 9106.

## Decision

- New and changed passwords are hashed with Argon2id: 64 MiB memory, 3 passes, one lane,
  16-byte random salt, and a 32-byte key. The hash is stored in PHC string format
  (`$argon2id$v=19$m=65536,t=3,p=1$<salt>$<key>`). The implementation is
  `golang.org/x/crypto/argon2`, from the module that already provides bcrypt, so no new
  dependency is added (DR-11).
- Existing bcrypt hashes keep working. After a successful password check at login, a hash
  that is bcrypt or uses other Argon2id parameters is replaced by a current Argon2id hash of
  the same password. The update applies only while the old hash is still stored, so a
  password change made in the meantime is not overwritten. The password itself does not
  change, so sessions stay valid (SC-017 does not apply) and no audit entry is written; the
  audit log never contains hashes anyway (SC-031).
- The password limit becomes 1024 bytes (SC-005). Argon2id has no input limit; the bound
  only caps the work per request. Longer passwords are still rejected with
  `422 password_too_long`, never truncated.
- At most two Argon2id computations run at the same time. Parallel logins wait instead of
  using more than 128 MiB of memory on the 4 GB host.
- Parameters in a stored hash outside fixed bounds (memory at most 256 MiB, at most 10
  passes, at most 8 lanes) are rejected before hashing, so a damaged row cannot make one
  login allocate unbounded memory.
- The dummy hash for unknown usernames (SC-009) is an Argon2id hash with the current
  parameters, so the unknown, password-less, and wrong-password paths cost the same.

## Consequences

- A login costs about 100–200 ms of CPU and 64 MiB of memory while it runs. The per-address
  limits (SC-008, SC-043) and the two-slot cap bound the load. Under a login flood, logins
  queue rather than exhaust memory (T-003).
- Until a player logs in again, the stored hash stays bcrypt. A bcrypt check is cheaper than
  the Argon2id dummy, so until then an unknown username and a known one with a bcrypt hash
  differ slightly in timing (SC-009). This ends as players log in.
- Old database snapshots still hold bcrypt hashes. They remain as strong as before.
