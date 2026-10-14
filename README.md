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

**Nur die vom Benutzer freigegebene Arbeitskopie darf bearbeitet werden.**
Original (strictly read-only / never target for sync):
https://docs.google.com/spreadsheets/d/1N5EgBixB1RNxZagK5d0ie_DMTcYmznpvW5UzuyEWgRU/edit

**Exklusives Arbeits-Sheet:**
https://docs.google.com/spreadsheets/d/10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8/edit

Der Sync wird ausschließlich für die Arbeitskopie zugelassen. Die ursprünglichen
17 Tabellenblätter müssen in der Kopie bestehen bleiben. Der ursprüngliche
Stand sollte bei Bedarf zusätzlich gesichert werden, bevor der Sync dort läuft.

**Konfiguration in der GUI (empfohlen):** Unter *Administration → Importe →
Fortlaufende Google-Sheets-Synchronisierung* die Synchronisierung aktivieren,
das Intervall und eine Begründung eingeben und einmalig die JSON-Datei des
Google-Servicekontos auswählen. Das Servicekonto erhält ausschließlich Zugriff
auf die freigegebene Kopie. Die Einstellungen werden dauerhaft in der DB
gespeichert und ohne Neustart angewendet. Zunächst **Automatisch abgleichen**
deaktiviert lassen, „Vorabprüfung ohne Änderungen“ und dann einen manuellen Lauf
auf einer Test-Datenbank ausführen; erst danach den automatischen Zeitplan
separat aktivieren.

**Alternativ (Bootstrap/Altinstallation):** Diese Umgebungsvariablen werden
weiterhin unterstützt, solange noch keine GUI-Konfiguration gespeichert wurde:

```sh
SHEET_SYNC_ENABLED=true
SHEET_SYNC_AUTORUN=false
SHEET_SYNC_SHEET_ID=10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8
SHEET_SYNC_INTERVAL=10m
SHEET_SYNC_REASON="Automatischer Abgleich auf der Arbeitskopie"
SHEET_SYNC_SERVICE_ACCOUNT_JSON='{"type":"service_account","client_email":"...","private_key":"..."}'
```

**Servicekonto-Schutz:** Die GUI zeigt die Zugangsdaten nach dem Hochladen
nicht mehr an. Sie werden verschlüsselt in SQLite abgelegt, der lokale
Schlüssel `<DATENBANKPFAD>.sheet-sync.key` ist separat (Dateimodus 0600)
gespeichert. **Für ein vollständiges Backup/Restore beide Dateien aufbewahren**
und vor unbefugtem Zugriff schützen. Der Schlüssel darf nicht ins Repository.
Eine gespeicherte GUI-Konfiguration hat Vorrang vor den alten Env-Werten.
`SHEET_SYNC_AUTORUN` ist auch beim Bootstrap ein separates Opt-in; die
Anmeldung und der normale Serverstart erfordern keine Sheet-Sync-Env-Werte.
Der [Aktivierungsleitfaden](docs/sheet-sync-activation.md) beschreibt die
Altbestandsprüfung für ICC25-35.

`SHEET_SYNC_SHEET_ID` darf nicht auf das Original gesetzt werden; der Server
prüft die freigegebene Arbeitskopie ausdrücklich und lehnt andere IDs ab.
Der periodische Sync importiert additive Historie und neue bekannte
Teilnehmer aus `ICC25 Raidsetup`; die Website exportiert Daten in eigene
`Website …`-Tabs **nur in der Kopie**. Konflikte verursachen einen Fehler.
Für die erste Einrichtung von Raidserie und geplantem Raid gilt weiterhin der
vorhandene manuelle Initialimport.

**Aktueller Raidsetup-Block (ADR 061):** Nach dem Erstimport übernimmt der
laufende Sync den ersten nummerierten `ICC25 Raidsetup`-Block in den einzigen
anstehenden, aktiven PLANNED/OPEN-Raid der importierten Serie (kein zusätzlicher
Raid). Beim Wechsel zum nächsten Raid wird zuerst geprüft, dass der
ursprünglich importierte Raid archiviert ist. Bevorzugt wird der
Vergleich mit der zuletzt gespeicherten Setup-Baseline; bei älteren Imports
ohne Baseline wird die letzte belegte archivierte Website-Aufstellung derselben
Serie verglichen. Die aktuelle Aufstellung muss sich tatsächlich
gegenüber der zuletzt archivierten unterscheiden; sonst wird die veraltete
Aufstellung nicht in den Nachfolger kopiert. Die Vorabprüfung zeigt den
Zielraid, seine ID, die Zahl neuer Teilnehmer und eventuelle Hinweise.
Unbekannte Spieler/Charaktere, geänderte Teilnahmen und mehrere aktive Raids
bleiben Konflikte. Schon importierte historische Teilnahmen bleiben unverändert.

**Sheet-Archivierung wird übernommen (ADR 059):** Ein Raid im `Raid Archive`
zählt auch auf der Website als abgeschlossen und archiviert. Der Sync übernimmt
Loot und die bereits verbuchten Teilnehmer-EP gemeinsam, mit append-only
Ausgleichsbuchungen; er vergibt keine zweite Abschlussprämie. Vorabprüfung zeigt
Archivübernahmen und EP-Differenzen. Fehlende alte Baselines/Original-Provenienz
können nach Prüfung des belegten Altbestands übernommen werden. Widersprüchliche
Website-Aufstellungen, unzugeordneter Loot oder parallele EP-Änderungen stoppen
alles. Wiederholungen erzeugen keine weiteren EP, Loot-Einträge oder Raidkopien.
Übernommene Sheet-Historie erzeugt keinen neuen Folge-Raid; ein schon geplanter
Nachfolger bleibt erhalten. Normales `Archivieren` eines aktiven Website-Raids
behält seine bestehende EP-/Lock-/Nachfolgerverarbeitung.

Im Tab `Loot Archive` wurde nur in der Arbeitskopie die Kopfzelle G1
(`Import ID`) ergänzt, damit neue Einträge eindeutige IDs erhalten können.
Bestehende Einträge wurden nicht umgeschrieben.

**Grenze:** Dies ist noch kein vollständiger bidirektionaler editierbarer
Abgleich. Löschungen, Umbesetzungen, nachträgliche historische Änderungen
und Änderungen an Website-Spiegeltabs werden nicht automatisch übertragen.
Kein stiller Last-Writer-Wins und kein automatisches Löschen von EP-/Loot-Historie.

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
