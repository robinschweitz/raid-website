# ADR 099: Off-server backups go encrypted to a Hetzner Storage Box

Status: Accepted.

## Context

ADR 070 and NFR-2.3 require a daily encrypted copy of the newest verified snapshot off the
server, with the key also held off the host (SC-034). The storage and the key holder were
deferred (OQ-14, OD-007, OD-020). On 2026-10-24 the owner booked a Hetzner Storage Box and
decided the open points. The owner said that someone reading a backup would be acceptable,
but a backup that can be overwritten would not.

## Decision

This ADR resolves OQ-14 and OD-007 and amends ADR 070 for the off-server copy.

- **Storage:** Hetzner Storage Box `storage-box-1` (BX11, location hel1). The production
  server is in fsn1, so the copy is in another data centre. The server writes through the
  sub-account `u700390-sub1`. That sub-account can only use SSH/SFTP, is reachable only from
  the Hetzner network, and is locked to the `raid-website` folder.
- **What is copied:** once a day, the newest verified snapshot with its SHA-256 sidecar and
  `/etc/raid-website.env`, packed into one tar file. The env file is included because a
  database restore needs the same secrets (later the SC-014 TOTP key).
- **Encryption:** the file is encrypted with GnuPG symmetric AES-256 (passphrase from
  `/etc/raid-website-backup/passphrase`, root-only) before it leaves the host. GnuPG is
  already on the host, so no new package is needed. The passphrase exists twice, on the
  server and with the owner (password manager). Confidentiality is not the main goal, so
  keeping the key on the host as well is acceptable (owner, 2026-10-24).
- **No overwrite:** the server only adds files. It uploads to a temporary name, renames to
  the final name (which fails if the name already exists), downloads the file again, and
  compares it. It never deletes or replaces a remote file, so the remote side has no
  retention. Daily Storage Box snapshots (10 kept, 05:30) are managed only through the
  Hetzner API, and the server holds no API token. So a compromised server cannot change
  copies that are already in a snapshot.
- **Restore test:** a monthly timer downloads the newest copy, checks both checksums,
  decrypts it, checks the database, and starts the installed server binary on a throwaway
  copy (development mode, loopback port, as `raidweb`). A failure fails the unit. The first
  restore on another machine (the owner's PC, with the owner's passphrase copy) passed on
  2026-10-24.
- The off-server steps are part of `deploy/bin/raid-website-backup` (`--offsite`,
  `--restore-test`), so they are reviewed with the other host scripts (SC-037).

## Consequences

- Maximum data loss stays one day (NFR-2.3). Remote files grow by about 4 MB a day. On 1 TB
  this needs no pruning for years; pruning would be a manual owner step.
- A failed upload or restore test is visible only as a failed systemd unit and in the
  journal. There is no notification channel.
- Losing the passphrase makes every remote copy unreadable. The owner keeps it outside the
  host.
