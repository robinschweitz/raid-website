# ADR 070: Production deployment and database snapshots

Status: accepted

The public reverse proxy is the only network entry point for the Go process.
Production instances bind the process to a loopback address, and the service
runs as the dedicated unprivileged `raidweb` user with systemd filesystem,
device, capability, and namespace restrictions.

SQLite backup files are made through SQLite's online backup API rather than
copying the database and WAL files. Each snapshot is integrity-checked,
gzip-compressed, atomically published with a SHA-256 sidecar, and retained as
48 hourly, 32 daily, 13 weekly, and 25 monthly recovery points. A scheduled
job independently decompresses and verifies the newest snapshot weekly.

Database snapshots stored on the same server are not disaster recovery. An
independent provider snapshot policy or off-server encrypted copy remains an
operational requirement and must be tested by restoring it.

Automatic polling, checkout, and building of a mutable Git branch as root is
not part of the production baseline. Deployments use reviewed revisions with a
backup, health check, and protected source branch.
