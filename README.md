# Raid-Website

WotLK Raid management (Armor Tokens, Enhance Points, Loot, Attendance). Modular monolith: Go backend (Bun + SQLite) + React/TypeScript frontend.

The UI is branded for **Bratwurst des Schicksals**: an original icy crest/citadel atmosphere,
navy/cyan panels, desktop sidebar, mobile navigation, saved class-colored roster details,
and a real-data dashboard. No external artwork, fake progression, or new UI dependencies.

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
  additions also require a reason. EP corrections append compensating entries; tokens derive from loot awards.
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
- EP is tracking-only: there is no website roll or standalone spend action. The shifted
  outside range is `1 + X` to `100 + X`, where `X = min(balance, 50)`. Only an enhanced
  loot winner pays, as part of staff recording that outside outcome.
- Migrations 006-008 add award methods, linked EP costs, request identities, and catalog
  associations. Migration 009 records each bundled-catalog startup initialization. Existing
  ledgers and historical free-text loot are preserved.

## Loot Workspace

Raid staff uses the raid's Loot section to choose the boss, dropped item, roster character,
and `MAIN`, `SECOND`, `ENHANCED`, or `BOE`. Main and Enhanced prevent the +10 archive reward;
Second and player BoE do not. Enhanced costs are computed server-side, with no amount selector.

Corrections preserve a same-winner enhanced cost on unrelated edits. Changing away from
Enhanced refunds it; changing the enhanced recipient refunds the old winner and charges
the new winner's current capped balance. The award, compensating entries, derived token
visibility, and audit commit together. Archived/cancelled raids remain read-only.

New enhanced POSTs require a stable, nonblank `request_id`; identical retries return the
same award instead of charging again. Loot DTOs use `award_method`, not a caller-controlled
`is_main_spec`. New manual recipients must identify a character in that raid's roster.


## Fortlaufender Google-Sheets-Abgleich (opt-in)

Der einmalige Import bleibt für die initiale Übernahme samt explizitem Serienplan
erhalten. Danach kann der Server die vier historischen Google-Sheet-Tabs und
das aktive `ICC25 Raidsetup` regelmäßig prüfen. Die bestehenden
Import-Provenienzen, Vorschau-Hashes und Konfliktregeln gelten weiter.

1. In der Google Cloud Console **Google Sheets API** aktivieren, ein **Servicekonto**
   erstellen und seine E-Mail-Adresse im Guild-Sheet als **Bearbeiter** freigeben.
   Die JSON-Zugangsdaten nur als Secret auf dem Server hinterlegen, **niemals**
   einchecken oder im Browser verwenden.
2. Diese Umgebungsvariablen setzen und den Server neu starten:

```sh
SHEET_SYNC_ENABLED=true
SHEET_SYNC_SHEET_ID=1N5EgBixB1RNxZagK5d0ie_DMTcYmznpvW5UzuyEWgRU
SHEET_SYNC_INTERVAL=10m
SHEET_SYNC_REASON="Automatischer Abgleich der Sheet-Archiv-Ergänzungen"
SHEET_SYNC_SERVICE_ACCOUNT_JSON='{"type":"service_account","client_email":"...","private_key":"-----BEGIN PRIVATE KEY-----\\n...\\n-----END PRIVATE KEY-----\\n"}'
```

Der Intervall muss mindestens 1 Minute betragen. Ohne `SHEET_SYNC_ENABLED=true`
bleiben automatische Jobs und Schreibzugriffe deaktiviert. Mit aktiviertem
Servicekonto werden auch private Sheets über die authentifizierte Google
Sheets API gelesen; die öffentliche gviz-Freigabe ist nicht mehr nötig.

Im Adminbereich **Importe** werden letzter Versuch, letzter erfolgreicher
Lauf und Konflikte angezeigt. **Jetzt synchronisieren** startet einen
manuellen Lauf. Der Server führt beim Start und anschließend regelmäßig
einen Abgleich aus; der letzte Status überlebt Neustarts. Die Endpunkte
`GET /api/v1/sheet-sync` und `POST /api/v1/sheet-sync/run` sind Admin-only.

**Konfliktsichere Richtungen:**

- **Google Sheet → Anwendung:** Neue historische Zeilen im `Raid Archive`,
  `Loot Archive`, `Charaktere`, `Punkte` laufen über Dry-Run und
  Merge mit Hashprüfung. Der aktuelle Raid wird aus dem ersten nummerierten
  A:F-Block in `ICC25 Raidsetup` ergänzt – **nur** bekannte, eindeutig
  zugeordnete Charaktere und nur bei exakt einem aktiven Raid in der
  importierten Serie. Der erste Raid samt Serie muss zuvor mit der
  bestehenden manuellen Importmaske eingerichtet sein.
- **Anwendung → Google Sheet:** Der Server spiegelt seine Daten in
  `Website Raids`, `Website Raidsetup`, `Website Teilnahmen`,
  `Website Loot`, `Website Charaktere`, `Website Punkte` und
  `Website Raidserien`. Diese Tabs sind als **nur lesbare Spiegel**
  gedacht; geänderte Inhalte erzeugen einen Konflikt statt überschrieben
  zu werden. Die vier ursprünglichen Archivtabs und das ursprüngliche
  Raidsetup werden **nicht** durch den Export verändert.

**Wichtige Grenze:** Dies ist noch **kein vollständiger bidirektionaler
Bearbeitungs-Sync**. Nachträgliche historische Änderungen, Ersetzungen/
Entfernungen von Raidmitgliedern sowie manuelle Bearbeitungen der Website-
Spiegelblätter müssen weiterhin mit den vorgesehenen auditierten
Website-Workflows korrigiert werden. Ein Konflikt stoppt den entsprechenden
Lauf. Daten oder bestehende EP-/Loot-Buchungen werden niemals automatisch
gelöscht oder überschrieben. Siehe ADR 057.

## Local Beutekatalog

`scripts/data/catalog-wotlk335.json` is the committed AtlasLoot/Wowhead WotLK dataset generated by
`scripts/build_loot_db_localized.py` with `scripts/lootcatalog.localized.json`. It currently covers
Icecrown Citadel with English canonical item stats and German display names/tooltips.

The server embeds that file and serves it as the only catalog. Startup mirrors its instances,
bosses, items, and drops into the foreign-key tables, records a report in
`loot_catalog_initializations`, and fails if the file or the mirror is invalid. There is no upload,
preview, or merge: **Beutekatalog** is a read-only view, and every authenticated role may read it.
Updating the catalog means committing a new file.

Source paths are normalized server-side into `NORMAL`, `HEROIC`, and `HARDMODE`. Relationships
that share boss, item, raid size, and mode are merged; their raw source variants remain available
in the API for diagnostics, but ordinary UI keeps them hidden.

Optional `catalog_instance_id` and `raid_size` associations can be configured on series
and active raids. Available sizes come from the bundled drops. New loot must use a bundled
`catalog_boss_id` and `item_id` for that instance and size; canonical names are derived
server-side. Unassociated raids cannot record new loot
(`catalog_context_required`). Unchanged historical identities are preserved by unrelated
corrections, not guessed from current catalog data.

Catalog reads: `/api/v1/loot-catalog`, `/instances`, `/bosses`, and `/items` beneath that
prefix. There are no catalog write endpoints.

To regenerate the catalog from the checked-in generator config, run:

```sh
python scripts/build_loot_db_localized.py scripts/lootcatalog.localized.json
```

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
