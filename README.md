# Raid-Website

WotLK Raid management (Armor Tokens, Enhance Points, Loot, Attendance). Modular monolith: Go backend (Bun + SQLite) + React/TypeScript frontend.

## Quick Start (dev)

- Backend: `go run ./cmd/server` (after setup)
- Frontend: `cd web && npm run dev`

## Build (production)

- `make build` runs `npm ci && npm run build` in `web/` (output: `internal/webui/dist/`) and then `go build ./...`.
- The compiled SPA is embedded into the binary and served by the Go server, so build the frontend before the backend.

## Quality Gates

- `gofmt` / `go vet` / `golangci-lint` / `go build ./...` / `go test ./...`
- TypeScript strict + ESLint
- Frontend regression tests: `npm test` in `web/` (Node's built-in test runner).

## Runtime Safety

- Production requires `ENV=production` and explicit nondefault `ADMIN_USERNAME` and
  `ADMIN_PASSWORD`. Startup fails on configuration, migration, or admin bootstrap errors.
- SQLite foreign keys are enabled automatically. Back up an existing database before
  deployment; migration `003_import_identity` adds import provenance and preview metadata.
- Attendance and loot edits require a reason and write an audit trail. Completed-raid
  additions also require a reason. EP and token corrections append compensating entries.
- `Archivieren` is the single finalization action: it awards EP, expires unused locks,
  and schedules the next future raid atomically. Archived history remains visible.
- Configure recurring raid series under Roster: name, leader, weekdays, week interval,
  local start time, and IANA timezone. Assign a series and scheduled start to active raids
  before archiving. No default guild schedule is provided.
- Attendance stores the character name, role, class, and actual spec played. New imports
  use Raid Archive's source values. Older missing snapshots are shown as unavailable,
  not guessed from current characters. Final archive requires a complete active roster.
- Incremental Loot Archive rows should have a unique, permanent `Import ID` column.
  Changed or ambiguous source rows produce a conflict report, never an overwrite.

## Correction API

Authenticated RAIDASSISTENT and higher may use:

- `PATCH /api/v1/raids/{id}/attendance/{recordID}` with changed attendance fields and `reason`.
- `PATCH /api/v1/raids/{id}/loot/{recordID}` with changed loot fields and `reason`.
- The existing attendance and loot POST routes, including `reason` for completed raids.

Cancelled and archived raids reject these writes. The frontend provides correction forms.
Unarchived historical COMPLETED records retain their audited correction workflow; new
finalized raids are read-only after archive.

## Raid Series API

Authenticated users can list `GET /api/v1/raid-series`. Admins create or patch series with
`name`, `raid_leader_id`, `weekdays` (0 Sunday through 6 Saturday), `interval_weeks`,
`start_time` (`HH:MM`), and `timezone` (IANA name).

Raid assistants and higher may use `PATCH /api/v1/raids/{id}` with `series_id` and/or
`scheduled_start` (RFC3339). Omitted fields stay unchanged. `POST /api/v1/raids/{id}/archive`
returns the finalized raid in `data` and its successor in `next_raid`. The old `/complete`
endpoint is removed. Existing completed history never triggers new historical rewards
or a recurrence chain. Migrations 004 and 005 preserve existing records without automatic
awards or invented schedules.

## Import API

Call `POST /api/v1/imports/dry-run` with `{"sheet_id":"..."}` first. Review the report,
then call `POST /api/v1/imports` with `{"sheet_id":"...","raw_hash":"<preview hash>"}`.
Include `"reason":"..."` when adding rows to an existing imported raid; these changes
use the same audited correction and ledger rules as manual corrections.
Both routes are Admin-only. A changed source requires a new preview; report errors prevent
all domain writes. Identical replay returns the saved report without reprocessing.

See ARCHITECTURE.md and docs/decisions/ for details.
