# Architecture

Modular monolith. Domains: players/characters, raids/attendance, loot, enhancepoints (ledger), tokens (ledger), roster (configurable ranks/groups), audit, legacyimport/export, httpapi (v1). `raidcompletion` and `raidcorrection` coordinate cross-domain writes through repository-owned transactions.

`raidseries` owns explicit recurring schedule configuration and local-calendar calculations.
`raidcompletion` implements final archival: lifecycle, EP, lock expiry, and successor linkage
share a transaction. Attendance stores per-raid name/class/spec snapshots, not live-character
presentation data. Archived raids remain visible history (ADR 021-022).

Principles:
- Domain-first, boring/explicit/predictable
- Append-only ledgers for EP/tokens/audit
- Archive by default, explicit delete only (Admin)
- Edits with audit trail
- UTC storage, client TZ display
- Server-side authz
