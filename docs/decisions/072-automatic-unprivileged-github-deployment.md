# ADR 072: Automatic unprivileged GitHub deployment

Status: accepted

Supersedes the manual-trigger portion of ADR 071.

The production host checks the protected `main` branch hourly with a randomized
delay. Each update fetches, tests, builds, snapshots the database, publishes,
and health-checks automatically. The earlier root polling job remains disabled.

The trust boundary from ADR 071 remains unchanged: the GitHub checkout and all
build commands run as `raiddeploy`. The builder may only start a fixed,
root-owned publishing unit. That unit separately validates the fixed build
paths and revision, installs the binary, and restarts the application as
`raidweb`.
Branch protection and review remain required because an automatically deployed
application revision can access application data.
