# ADR 071: Manual unprivileged GitHub deployment

Status: accepted

Production updates are fetched from the protected `main` branch only when an
operator starts `raid-website-update.service`. There is no polling timer.

The checkout, package installation, tests, and compilation run as the
dedicated `raiddeploy` account. The account may invoke one root-owned publisher
through sudo with a fixed binary path. The publisher takes a verified database
snapshot, atomically replaces the root-owned binary, restarts the unprivileged
application service, and requires a loopback health check. It never executes
the build as root.

This limits a compromised repository revision to the build account and the
application's existing data permissions. It does not make an unreviewed
revision safe to deploy: `main` must remain protected and deployment stays a
deliberate operator action.
