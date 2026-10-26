# Production Deployment

The service uses a non-login `raidweb` account, stores mutable data in
`/var/lib/raid-website`, and accepts HTTP only on `127.0.0.1:8080`. nginx is
the sole public entry point and terminates TLS.

## Initial setup

Run these commands as root on Debian after copying or cloning this repository.
They intentionally do not create credentials or issue a TLS certificate; both
actions need owner-provided values.

```sh
apt-get update
apt-get install -y ca-certificates curl git nginx certbot python3 python3-certbot-nginx nodejs npm golang-go sudo unattended-upgrades
id raidweb >/dev/null 2>&1 || adduser --system --group --home /nonexistent --no-create-home raidweb
id raiddeploy >/dev/null 2>&1 || adduser --system --group --home /var/lib/raiddeploy --no-create-home raiddeploy
install -d -o root -g root -m 0755 /opt/raid-website
install -d -o raidweb -g raidweb -m 0700 /var/lib/raid-website
install -d -o raiddeploy -g raiddeploy -m 0700 /var/lib/raiddeploy
install -d -o raiddeploy -g raiddeploy -m 0755 /opt/raid-website/source /opt/raid-website/build
install -d -o root -g root -m 0700 /var/backups/raid-website
install -d -o root -g root -m 0755 /var/log/raid-website
install -d -o www-data -g www-data -m 0755 /var/www/certbot
install -m 0644 deploy/systemd/raid-website.service /etc/systemd/system/raid-website.service
install -m 0644 deploy/systemd/raid-website-backup.service /etc/systemd/system/raid-website-backup.service
install -m 0644 deploy/systemd/raid-website-backup.timer /etc/systemd/system/raid-website-backup.timer
install -m 0644 deploy/systemd/raid-website-backup-verify.service /etc/systemd/system/raid-website-backup-verify.service
install -m 0644 deploy/systemd/raid-website-backup-verify.timer /etc/systemd/system/raid-website-backup-verify.timer
install -m 0644 deploy/systemd/raid-website-backup-offsite.service /etc/systemd/system/raid-website-backup-offsite.service
install -m 0644 deploy/systemd/raid-website-backup-offsite.timer /etc/systemd/system/raid-website-backup-offsite.timer
install -m 0644 deploy/systemd/raid-website-backup-restore-test.service /etc/systemd/system/raid-website-backup-restore-test.service
install -m 0644 deploy/systemd/raid-website-backup-restore-test.timer /etc/systemd/system/raid-website-backup-restore-test.timer
install -m 0644 deploy/systemd/raid-website-update.service /etc/systemd/system/raid-website-update.service
install -m 0644 deploy/systemd/raid-website-update.timer /etc/systemd/system/raid-website-update.timer
install -m 0644 deploy/systemd/raid-website-publish.service /etc/systemd/system/raid-website-publish.service
install -m 0644 deploy/systemd/raid-website-reboot-if-required.service /etc/systemd/system/raid-website-reboot-if-required.service
install -m 0644 deploy/systemd/raid-website-reboot-if-required.timer /etc/systemd/system/raid-website-reboot-if-required.timer
install -m 0755 deploy/bin/raid-website-backup /usr/local/sbin/raid-website-backup
install -m 0755 deploy/bin/raid-website-update /usr/local/sbin/raid-website-update
install -m 0755 deploy/bin/raid-website-publish /usr/local/sbin/raid-website-publish
install -m 0755 deploy/bin/raid-website-configure-ufw /usr/local/sbin/raid-website-configure-ufw
install -o root -g root -m 0644 deploy/raid-website-update.env /etc/raid-website-update.env
install -o root -g root -m 0644 deploy/raid-website-backup.env /etc/raid-website-backup.env
install -o root -g root -m 0440 deploy/sudoers.d/raid-website-publish /etc/sudoers.d/raid-website-publish
visudo -cf /etc/sudoers.d/raid-website-publish
install -m 0644 deploy/apt/20auto-upgrades /etc/apt/apt.conf.d/20auto-upgrades
install -m 0644 deploy/apt/52raid-website-unattended-upgrades /etc/apt/apt.conf.d/52raid-website-unattended-upgrades
install -m 0644 deploy/sysctl.d/60raid-website-hardening.conf /etc/sysctl.d/60raid-website-hardening.conf
sysctl --system
install -d -m 0755 /etc/ssh/sshd_config.d
install -m 0644 deploy/sshd_config.d/raid-website.conf /etc/ssh/sshd_config.d/raid-website.conf
sshd -t
systemctl daemon-reload
systemctl enable --now unattended-upgrades raid-website-backup.timer raid-website-backup-verify.timer raid-website-update.timer raid-website-reboot-if-required.timer
systemctl reload ssh
/usr/local/sbin/raid-website-configure-ufw
```

Create `/etc/raid-website.env` with mode `0600`, owned by root. Production
requires `ENV=production`, `DATABASE_URL`, `ADMIN_USERNAME`,
`ADMIN_PASSWORD`, and `PUBLIC_ORIGIN`, the HTTPS origin of the site without a
path (for example `https://raid.example.org`); the server refuses to start
without it, because the WebAuthn relying party comes only from it (SC-007). Also
set `COOKIE_SECURE=true` and `TRUST_PROXY_HEADERS=true`. Configure nginx and request the certificate before
starting the service.

Push notifications (FR-32) are optional and stay off until both
`VAPID_PRIVATE_KEY` and `VAPID_SUBJECT` are set (SC-054). Create the key once on
any machine with Go and add both lines to `/etc/raid-website.env`; the
off-server copy then backs it up with the env file (SC-034):

```sh
go run ./cmd/vapid-key            # prints VAPID_PRIVATE_KEY=...
echo 'VAPID_SUBJECT=mailto:you@example.com' >> /etc/raid-website.env
systemctl restart raid-website.service
```

The nginx templates start with HTTP only, use a webroot ACME challenge, and
then replace the site with the TLS reverse-proxy configuration:

```sh
install -m 0644 deploy/nginx/raid-website-http.conf /etc/nginx/sites-available/raid-website
ln -s /etc/nginx/sites-available/raid-website /etc/nginx/sites-enabled/raid-website
nginx -t && systemctl reload nginx
certbot certonly --webroot --webroot-path /var/www/certbot --email you@example.com --agree-tos --no-eff-email -d bratwurst-des-schicksals.de -d www.bratwurst-des-schicksals.de
install -m 0644 deploy/nginx/raid-website.conf /etc/nginx/sites-available/raid-website
nginx -t && systemctl reload nginx
```

Build and install the application only after the frontend build has completed:

```sh
make build
install -o root -g root -m 0755 bin/server /opt/raid-website/server
systemctl enable --now raid-website.service
curl --fail http://127.0.0.1:8080/api/v1/health
```

## Database recovery

The timer creates an hourly, transactionally consistent SQLite snapshot using
Python's SQLite backup API, checks `PRAGMA integrity_check`, compresses it with
gzip, and writes a SHA-256 sidecar. Retention is 48 hourly, 32 daily, 13
weekly, and 25 monthly snapshots. The weekly verification job decompresses and
checks the newest snapshot again.

Run a snapshot or verification on demand:

```sh
systemctl start raid-website-backup.service
systemctl start raid-website-backup-verify.service
```

Restore only during an outage, after preserving the damaged database:

```sh
systemctl stop raid-website.service
cp -a /var/lib/raid-website/raid.db /var/lib/raid-website/raid.db.before-restore
gzip -cd /var/backups/raid-website/raid.db-YYYYMMDDTHHMMSSZ.sqlite3.gz >/var/lib/raid-website/raid.db
chown raidweb:raidweb /var/lib/raid-website/raid.db
chmod 0600 /var/lib/raid-website/raid.db
systemctl start raid-website.service
```

## Off-server copy

Local snapshots do not survive the loss of the server. Once a day
`raid-website-backup-offsite.timer` copies the newest verified snapshot, its
SHA-256 sidecar, and `/etc/raid-website.env` to the Hetzner Storage Box
`storage-box-1` (hel1, the server is in fsn1). The files are packed into one
tar file and encrypted with GnuPG (AES-256) before they leave the host (NFR-2.3,
SC-034, ADR 099). The server only adds files there. Each copy is uploaded under
a temporary name, renamed without replacing, downloaded again, and compared.
Nothing remote is ever deleted or overwritten by the server. The Storage Box takes
its own daily snapshot (05:30, 10 kept); only the Hetzner API can change those,
and the server has no API token.

On the first day of each month `raid-website-backup-restore-test.timer`
downloads the newest copy, checks both checksums, decrypts it, checks the
database, and starts `/opt/raid-website/server` on a throwaway copy
(development mode, `127.0.0.1:18080`, as `raidweb`). A failed upload or restore
test fails its unit; check with `systemctl --failed` and
`journalctl -u raid-website-backup-offsite.service -u raid-website-backup-restore-test.service`.

Root-only files in `/etc/raid-website-backup` (never commit them):

- `passphrase`: the GnuPG passphrase. The owner keeps a second copy in the
  password manager. Without it no remote copy can be read.
- `id_ed25519`: the SSH key of the Storage Box sub-account `u700390-sub1`
  (SSH only, Hetzner network only, folder `raid-website`).
- `known_hosts`: the pinned Storage Box host key (ED25519
  `SHA256:XqONwb1S0zuj5A1CDxpOSuD2hnAArV1A3wKY7Z3sdgM`).

Set up a new host (sub-account password from the Hetzner Console, used once):

```sh
install -d -o root -g root -m 0700 /etc/raid-website-backup
cd /etc/raid-website-backup
ssh-keygen -q -t ed25519 -N "" -C raid-website-backup -f id_ed25519
ssh-keyscan -p 23 -t ed25519 u700390-sub1.your-storagebox.de >known_hosts
ssh-keygen -lf known_hosts   # must match the fingerprint above
cat id_ed25519.pub | ssh -p 23 -o UserKnownHostsFile=known_hosts u700390-sub1@u700390-sub1.your-storagebox.de install-ssh-key
(umask 077; printf '%s\n' 'PASSPHRASE FROM THE PASSWORD MANAGER' >passphrase)
chmod 0400 id_ed25519 passphrase
systemctl enable --now raid-website-backup-offsite.timer raid-website-backup-restore-test.timer
systemctl start raid-website-backup-offsite.service raid-website-backup-restore-test.service
```

Restore after losing the server: on a host in the Hetzner network (a new
server), fetch a copy with the sub-account. The Storage Box is not reachable
from outside, so in the Hetzner Console either enable external access for a
moment or restore a Storage Box snapshot. Then decrypt it with the
passphrase:

```sh
gpg --batch --pinentry-mode loopback --passphrase-file passphrase -d -o bundle.tar raid-website-YYYYMMDDTHHMMSSZ.tar.gpg
tar -xf bundle.tar   # raid.db-*.sqlite3.gz, its .sha256, raid-website.env
sha256sum -c raid.db-*.sqlite3.gz.sha256
```

Install `raid-website.env` as `/etc/raid-website.env` (root, 0600), then
follow "Restore only during an outage" above with the decrypted snapshot.

## Deployment safety

The root-run scripts in `deploy/bin/` are reviewed like code (SC-037): change
them only through a pull request, then install them with the `install` lines
from "Initial setup". The installed copies must match the repository; check
with:

```sh
for f in deploy/bin/*; do cmp "$f" "/usr/local/sbin/$(basename "$f")"; done
```

Protect `main` in GitHub and review its commits before merging. The update
service fetches only the approved public repository and `main`, builds and
tests as `raiddeploy`, takes a database snapshot, then publishes only after a
health check. `raid-website-update.timer` runs this process hourly with a
randomized delay. The timer is the only automatic deployment mechanism.
Push the production listener hardening (`LISTEN_ADDR`) before a newer GitHub
revision is available; the updater intentionally refuses a revision that lacks
this guard.

The build uses the Go toolchain named in `go.mod` (`toolchain` line, SC-045):
`GOTOOLCHAIN=auto` in `raid-website-update.service` lets Go download that
version into `/var/lib/raiddeploy/go` even when the installed Go is older.
After changing the unit, reinstall it and check the running binary:

```sh
install -m 0644 deploy/systemd/raid-website-update.service /etc/systemd/system/raid-website-update.service
systemctl daemon-reload
go version /opt/raid-website/server   # shows go1.26.10 or newer
```

```sh
systemctl start raid-website-update.service
journalctl -u raid-website-update.service -n 100 --no-pager
cat /opt/raid-website/current-revision
systemctl list-timers raid-website-update.timer
```

Keep the SSH configuration key-only; retain console access or a tested SSH key
before applying it. APT fetches and installs updates daily, but does not reboot
the host immediately. The separate reboot timer restarts only when required at
03:00 Europe/Berlin.

## Security events in the journal

The application writes one JSON line per security event to stderr, which
systemd stores in the journal (SC-041, OD-013). Events: `login_failed`,
`login_blocked`, `mfa_failed`, `mfa_challenge_invalid`, `csrf_rejected`,
`password_changed`, `account_changed` (Admin change of a login, password or
role), `mfa_enabled`, and `mfa_disabled`. Each line has `time`, `event`,
`client_address`, `request_id`, and a `player_id` or normalised `username`;
it never holds a password, code, or token. List them with:

```sh
journalctl -u raid-website.service --since "7 days ago" --no-pager | grep '"msg":"security_event"'
```

The journal must keep them 30 days. Install the retention setting once and
restart journald:

```sh
install -d /etc/systemd/journald.conf.d
printf '[Journal]
MaxRetentionSec=30day
' > /etc/systemd/journald.conf.d/raid-website.conf
systemctl restart systemd-journald
```

## Incident plan and secret rotation

Use this plan when you suspect a compromise: unknown logins or role changes in
the audit log, security events in the journal, a leaked secret, or a bad
deployment. The owner is the only person who acts. There is no in-app "log
everyone out" button (SC-044, OD-015). Run the commands as root on the host.
Never write a secret into this repository or a chat; generate it on the host
and put it only in `/etc/raid-website.env` or Cloudflare secrets (SC-033).

1. **Detect.** Look at what happened before you change anything:

   ```sh
   journalctl -u raid-website.service --since "7 days ago" --no-pager
   journalctl -u raid-website-update.service --since "7 days ago" --no-pager
   cat /opt/raid-website/current-revision
   ```

   The Admin audit log in the app shows data changes. The journal keeps
   security events for 30 days (SC-041); see "Security events in the journal"
   below.

2. **Contain: pause automatic deploys.** Stop the hourly update so a changed
   `main` is not installed while you work:

   ```sh
   systemctl disable --now raid-website-update.timer
   ```

   Start it again with `systemctl enable --now raid-website-update.timer`
   once the incident is closed.

3. **Contain: end all sessions.** Every session lives in the `sessions` table.
   Deleting every row logs everyone out, including you; the first request
   after that returns 401 (SC-016). Snapshot first, then delete:

   ```sh
   systemctl start raid-website-backup.service
   python3 -c "import sqlite3; c = sqlite3.connect('/var/lib/raid-website/raid.db'); c.execute('PRAGMA busy_timeout=5000'); print(c.execute('DELETE FROM sessions').rowcount, 'sessions deleted'); c.commit()"
   ```

   The service may keep running; SQLite serializes the write. Pending MFA
   challenges expire after 5 minutes (SC-010) and need no action.

4. **Rotate secrets.** Do this for every secret that may be exposed. After each
   change to `/etc/raid-website.env`, run `systemctl restart raid-website.service`
   and `curl --fail http://127.0.0.1:8080/api/v1/health`.

   - **`ADMIN_PASSWORD`.** The server only creates the bootstrap Admin when the
     account does not exist and never changes an existing one (FR-2.10).
     Changing the variable alone therefore does not change the live password.
     Set a new strong value in `/etc/raid-website.env` (so a rebuilt database
     gets it), then change the live password: log in as an Admin and change the
     own password in the app, which ends all other sessions (FR-2.13). If you
     are locked out, use the server-side reset code command `cmd/reset-code`
     (SC-051, not built yet); until it exists, restore access from a verified
     snapshot (step 5). Any other Admin whose password may be known gets a new
     password in the app (FR-2.14).
   - **Worker token (`RISING_GODS_PROXY_TOKEN`).** The token exists twice and
     both must match: the Worker secret `UPSTREAM_AUTH_TOKEN` in Cloudflare and
     `RISING_GODS_PROXY_TOKEN` in `/etc/raid-website.env`. Generate a value with
     `openssl rand -hex 32`, set the Worker secret first
     (`cd workers && npx wrangler secret put UPSTREAM_AUTH_TOKEN`, paste the
     value), then update the env file and restart. Lookups through the Worker
     fail between the two steps (502 `rising_gods_unavailable`); registration
     and login are unaffected.
   - **Cloudflare API token (GitHub secret `CLOUDFLARE_API_TOKEN`).** It is used
     only by `.github/workflows/deploy-rising-gods-proxy.yml` to deploy the
     Worker. In the Cloudflare dashboard roll or delete the old token and create
     a new one with only the Workers edit permission for this account, then
     replace the GitHub repository secret of the same name. Check with a manual
     run of that workflow (`workflow_dispatch`).
   - **TOTP key (`TOTP_ENCRYPTION_KEY`).** SC-014 specifies the key and its
     rotation (`TOTP_ENCRYPTION_KEYS_PREVIOUS`, re-encryption at startup), but it
     is **not built**: TOTP secrets are still stored as plaintext and the
     variable is not read. Until SC-014 is implemented there is no key to
     rotate. If the database file or a snapshot leaked, treat every TOTP secret
     as exposed: ask each user to disable and set up TOTP again (users with a
     lost second factor get a password reset, FR-3.11). Once SC-014 is built,
     rotate as it describes and keep the old key with the backup key material
     until the last snapshot that uses it has expired.
   - **VAPID key (`VAPID_PRIVATE_KEY`).** Create a new key with
     `go run ./cmd/vapid-key`, replace the line in `/etc/raid-website.env`, and
     restart the service. Every existing push subscription stops working with
     the old key; players switch notifications on again in their account tab
     (SC-054). Delete the stale rows with
     `sqlite3 /var/lib/raid-website/raid.db 'DELETE FROM push_subscriptions;'`.
   - **Deploy keys.** The updater fetches the public repository over HTTPS and
     uses no deploy key or token (see "Deployment safety"). If you later add
     one, add its rotation step here. SSH access is key-only: remove unknown
     keys from `~/.ssh/authorized_keys` of every account that can log in
     (SC-035), and rotate the host-side Cloudflare or GitHub account passwords
     and 2FA if they may be exposed.

5. **Restore from a verified snapshot** if data or the binary may have been
   tampered with. Pick a snapshot from before the incident, confirm it with
   the SHA-256 sidecar next to it (`sha256sum -c <sidecar file>` inside
   `/var/backups/raid-website`), and follow "Database recovery" above. If the
   host itself is lost or its snapshots are not trustworthy, use an older
   copy from the Storage Box or one of its snapshots ("Off-server copy"):

   ```sh
   ls -l /var/backups/raid-website/
   systemctl start raid-website-backup-verify.service
   ```

   A restored database brings back old sessions and secrets; repeat step 3
   (delete all sessions) and step 4 afterwards. Rebuild the binary only from a
   reviewed commit of `main`, then check `cat /opt/raid-website/current-revision`.

6. **Review and close.** Write down what happened, how it was found, and what
   was rotated. Check the audit log for changes made during the incident and
   correct them as Admin. Re-enable `raid-website-update.timer`. Record any
   spec gap you found in `SECURITY.md` or `THREAT_MODEL.md`.
