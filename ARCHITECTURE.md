# Architecture

Modular monolith. Domains: players/characters, raids/attendance, loot, enhancepoints (ledger), tokens (ledger), roster (configurable ranks/groups), audit, legacyimport/export, httpapi (v1).

Principles:
- Domain-first, boring/explicit/predictable
- Append-only ledgers for EP/tokens/audit
- Archive by default, explicit delete only (Admin)
- Edits with audit trail
- UTC storage, client TZ display
- Server-side authz
