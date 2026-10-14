# AI Working Agreement — raid-website

Single entry point for AI agents (and humans) working in this repository. It records the
requirements and constraints the project owner defined **before** implementation started, so that
future agents do not re-litigate them or silently change them.

**If a rule is not listed here, you do not know it. Do not invent it — ask.**

Accepted decisions are current project memory, not untouchable law. Follow them while they are
current. If the owner says an old decision is wrong, incomplete, or no longer wanted, update the
decision record in the same change: add a new ADR that supersedes or amends the old one, and update
this file when it summarizes the changed rule. Do not force an implementation just because an old ADR
exists; ask, then record the new decision explicitly (ADR 043).

Authoritative sources, in order of precedence:

1. This file (owner-level requirements and constraints)
2. `docs/decisions/*.md` (one accepted decision per file)
3. `ARCHITECTURE.md` (module map and principles)
4. `CODING_RULES.md` (defines the coding rules)
5. `README.md` (commands)

---

## What this project is

A WoW WotLK guild raid-management website: raids, attendance, loot, Enhance Points, Armor Tokens,
polls, roster configuration, the legacy Google Sheet import, and opt-in continuous sheet reconciliation (ADR 057).

Architecture is a **modular monolith** (ADR 001): Go backend + SQLite via Bun (ADR 002), React +
TypeScript frontend built with Vite. One binary serves both the JSON API and the compiled SPA.

---

## Non-negotiable engineering rules

These came from a strict, German-language engineering-standards document that governs the whole
codebase. They are not stylistic preferences.

`CODING_RULES.md` elaborates these rules for day-to-day coding (layers, errors, migrations, tests,
frontend, git) in 16 sections. It never overrides the list below; where the two disagree, this file
wins and the difference is fixed here first.

1. **Boring, explicit, predictable.** No clever abstractions, no hidden magic, no speculative
   generality. Prefer long and obvious over short and clever. Name things after the domain.
2. **Domain-first.** Business logic lives in `internal/<domain>` packages, never in HTTP handlers.
   Handlers only decode, delegate, and encode. Repositories own the ORM; Bun never appears outside
   the repository layer (ADR 002).
3. **"Do not guess. Ask."** If a business rule is ambiguous, stop and ask the owner. Never invent
   numbers, thresholds, statuses, or workflows.
4. **Append-only ledgers.** Enhance Points and the audit log are append-only.
   EP balances are computed by summing transactions. Never `UPDATE` or delete history; a correction is
   a new, compensating entry (ADR 004). Armor Tokens are derived from loot awards, not tracked in a ledger (ADR 041).
5. **Never silently overwrite.** Legacy import and any bulk write must produce a report and must not
   clobber existing data (ADR 008).
6. **Archive by default.** History-affecting records are archived (`archived_at`), never deleted.
   A hard delete must be explicitly requested by a human and is Admin-only (ADR 007).
   For raids, archiving means final processing, not removal or hiding (ADR 021).
7. **Edits are audited.** Changes to history-affecting records (loot, attendance, character renames)
   require an actor and write an audit log entry. A reason is mandatory only for archived raid changes
   (ADR 032 supersedes the reason requirement in ADR 006). Numeric corrections prefer new
   adjustment transactions over mutating values.
8. **UTC in storage, local time at display.** Persist all timestamps in UTC; the frontend formats in
   the user's timezone. Import must convert German sheet timestamps explicitly (ADR 003).
9. **Authorization is server-side.** The client never decides permissions. Every mutation is checked
   against the caller's role; hiding a button is not access control (ADR 010).
10. **Explicit configuration, not hard-coding.** Guild ranks and raid groups are DB rows that an
    Admin can change (ADR 009). Do not bake such values into code or migrations.
11. **Errors are explicit.** Domain packages return typed errors (e.g. `ErrLockNotAllowed`); handlers
    map them to HTTP status + stable machine-readable `code`. No silent `nil`-swallowing.
12. **Tests and gates.** New behaviour needs a test. Code must pass `gofmt`, `go vet`,
    `go build ./...`, `go test ./...`, TypeScript strict and ESLint before being considered done.

---

## Stack (fixed — do not substitute)

- Go 1.26, `net/http` with the Go 1.22+ method-pattern `ServeMux` (`mux.HandleFunc("GET /api/v1/...")`).
- Bun ORM v1.3 with the SQLite dialect (`sqlitedialect` + `sqliteshim` + `modernc.org/sqlite`).
- SQL migrations as numbered `.up.sql` / `.down.sql` files embedded via `go:embed`
  (filename regex: `^(\d{1,14})_([0-9a-z_\-]+)\.`).
- Frontend: React 18 + TypeScript (strict) + Vite. No state-management or HTTP library was added, so
  the client is a small hand-rolled fetch wrapper plus a hash router. **Adding a dependency is a
  decision for the owner — ask first.**
- Password hashing: `golang.org/x/crypto/bcrypt`.

---

## Domain rules the owner confirmed

Every item here was explicitly decided by the owner. Do not "improve" on them.

### 4.1 Players and characters
- A player may own multiple characters. Each character has a class, a main spec, and a second spec.
- `is_main` marks the character the player currently raids on as main — it is a preference flag, not
  a uniqueness constraint.
- Character names are mutable because players rename in game. Therefore **no DB-level global
  `UNIQUE(name)`**; records are addressed by stable IDs and renames are recorded in name history
  (ADR 013).
- ADMIN may merge an active source account into a distinct target with a required reason. The target
  keeps credentials, role, and MFA; source authentication is revoked, all references transfer, and
  the source account is permanently deleted (ADR 056 supersedes ADR 050).
- Raid attendance selection is character-first. RAIDASSISTENT and higher may create a new
  character and optionally its new player through atomic raid quick-add (ADR 037). New player
  names default to the character name and are editable; these players use the existing GUEST
  default without login credentials. General account administration remains Admin-only.
- Interactive Rising-Gods character lookup is restricted to Alliance level-80 characters. Pasted
  normal Rising-Gods profile URLs may be resolved server-side to the faster profiler load endpoint;
  broad name search remains available and uses targeted filtered Rising-Gods queries with
  caching/coalescing where practical. Broad Rising-Gods listviews are capped and must not be treated
  as complete character indexes (ADR 044).

### Raids
- Lifecycle: `PLANNED` → `OPEN` → `COMPLETED`, with cancellation as `CANCELLED`.
  `Archivieren` is the single final action: it sets completed and archived timestamps together.
  There is no separate completion action (ADR 021 supersedes the former raid archive semantics).
- Archiving finalizes roster/loot processing, awards EP, expires unused locks, and schedules the
  next raid atomically. Repeated requests must neither award twice nor create duplicate successors.
- Archived raids remain visible as history. Raid staff can correct roster and loot or add missing
  entries with a mandatory reason; archival timestamps, finalization, and successors stay fixed (ADR 032).
- Each recurring raid series explicitly configures name, leader, weekday(s), week interval,
  local start time, and IANA timezone. No guild schedule is seeded or hard-coded.
- Recurrence weeks are anchored to the current raid's local Monday week. Archiving chooses the
  next future occurrence, skipping elapsed dates, and preserves local wall time across DST.
  Ambiguous/nonexistent local times fail explicitly rather than silently shifting the schedule.
- A preplanned eligible next occurrence is reused without replacing its data. A conflicting
  finalized/cancelled occurrence or another predecessor is reported, not overwritten.
- Existing stored completed raids are historical: archival never recreates their EP or starts
  a new recurrence chain. Existing active raids can be assigned a series and scheduled start.

#### Future raid signups (ADR 033)
- RAIDASSISTENT and higher explicitly open/close signups independently of raid lifecycle.
- MEMBER and higher may submit multiple own characters, each with any valid class spec.
- Pending choices can be edited/withdrawn while open; accepted choices are staff-managed.
- Only one character per player is accepted, automatically creating the roster snapshot.
- Alternatives remain visible; staff can explicitly switch the roster slot.
- Signups close at the scheduled start (server-enforced) or manually earlier. Staff may
  continue accepting choices in active raids after closing. All changes are audited.

### Enhance Points (EP) — the core rule
- **+10 EP** for a player who received **no main-spec loot** in that raid.
- Awarded **only by final archiving**, which moves the raid to `COMPLETED` (ADR 021).
- **No EP at all if the raid is `CANCELLED`** (ADR 015).
- One award per (player, raid) — idempotent.
- Balance is `SUM(amount)` over the append-only `enhance_point_transactions` (ADR 004).

### Recorded enhanced loot (ADR 023-024)
- The website tracks EP and outside raid outcomes; it never generates a roll or exposes a
  player spend button, amount selector, or standalone EP-use action.
- The actual outside roll is a shifted 1-100 roll: `1 + X` through `100 + X`, with
  `X = min(balance, 50)`. Only an enhanced winner pays; losing rolls cost nothing.
- Raid staff records winning loot as `MAIN`, `SECOND`, `ENHANCED`, or `BOE`. An enhanced
  award atomically debits `X`, requires `X >= 1`, and counts as main-spec loot. The amount
  cannot be selected or split. A player BoE award does not affect main-spec EP eligibility.
- Loot recipients are characters in that raid's roster. Ordinary main/second awards and
  token derivation retain their rules. Active locks still award main-spec for zero EP.
- Coupled corrections preserve the historical cost for the same enhanced winner on
  unrelated edits. Changing away from enhanced refunds that cost; changing the enhanced
  recipient refunds the old winner and charges `min(new winner's current balance, 50)`.
  All corrections are actor-audited, require a reason for archived raids, and append compensating transactions atomically.
- Existing ledger history is retained, not recalculated under the revised rules.

### Item locks
- A lock requires `balance >= 150` but **costs 0 EP** (ADR 012).
- A player may hold **multiple** simultaneous locks.
- If the locked item drops in that raid, it is auto-awarded to the locker **as a main-spec award**.
- If it does not drop, the lock **expires at raid completion** with no loss (ADR 012).

### Attendance
- A **substitute** is a normal player; substitute and regular participants are treated identically.
- **Bench** participants follow the **same EP rule** as raiders: they earn +10 if they got no
  main-spec loot (ADR 016).
- The raid roster remains editable after archival with a reason. It stores the actual character name, role,
  class, and spec played in that raid, independently of later live-character changes (ADR 022).
- Archive rejects an incomplete active roster. Legacy unknown snapshots are not fabricated;
  new sheet imports preserve Raid Archive's own character name, role, class, and spec.

### Armor Tokens
- Tokens are derived from awarding the three specific loot items
  `Weiheabzeichen des Beschützers`, `Weiheabzeichen des Bezwingers`,
  `Weiheabzeichen des Eroberers` (ADR 014).
- There is no token ledger or separate token balance. Token information is a filtered view of loot awards (ADR 041 supersedes ADR 005).
- The legacy sheet's "Tokens gesammelt" total is **reconciliation-only** and must never be written
  over derived data.

### RaidSync addon exchange (ADR 045)
- The in-game addon RaidSync (separate `wow-addons` repository) exchanges copy-and-paste text with the
  website: an invite export of the saved roster (current character names, bench optional), a roster
  export of who was actually in the raid, and a loot export recorded in game by master loot, including
  trades of items the loot master held.
- Roster import is preview then apply, RAIDASSISTENT and up, only for planned/open unarchived raids. It
  adds selected known characters through the ordinary attendance path; unknown characters go through
  the Rising-Gods quick-add, never automatic creation.
- Loot import is preview then apply with the same permissions and raid states. Staff choose the award
  method (default `MAIN`, armor tokens `SECOND`) and may override the recorded recipient with any roster
  character. Lines go through the ordinary loot path (catalog, locks, Enhanced costs) with request IDs
  derived from the addon's entry ID, so repeated imports create nothing new even after a trade changed
  the recipient. `raidsync-` request IDs are raid-scoped (`loot.RaidScopedRequestPrefix`, checked in the
  write transaction and by `idx_loot_raid_request`), unlike ordinary per-actor request IDs.

### Loot, attendance, and character corrections
- Corrections are actor-audited with before/after snapshots. Reasons are mandatory for archived
  raid history only; live-character changes never rewrite raid snapshots (ADR 032).

### Roster configuration
- Guild ranks and raid groups are configurable rows in the database, not hard-coded constants
  (ADR 009).

### Legacy Google Sheet import and ongoing sync (ADR 057)
- ADR 061 treats the first numbered `ICC25 Raidsetup` block as the current
  roster for the single active imported-series raid. Moving to a successor
  requires the previous import raid archived and a changed roster against
  a proven archived setup baseline or the last archived website roster on
  upgrades lacking a baseline. Never copy stale rosters, invent raids,
  edit old raids or silently remove participants.
- ADR 060 permits Admin-only GUI configuration of the authorized-copy sync, with
  persistently encrypted service-account JSON and immediate runtime changes. The
  separate automatic opt-in, read-only preflight, provenance checks, and hard-coded
  prohibition on syncing the original Sheet remain in force. The local 0600 key
  file must accompany the SQLite DB during backups/restores.
- ADR 058 adds safe activation: API-only unless `SHEET_SYNC_AUTORUN=true`, an
  Admin-only read-only preflight, persisted source baselines, atomic domain
  reconciliation and a single atomic Sheets snapshot batch. Missing old baselines
  and ambiguous cutover data block activation unless the proven archive adoption
  in ADR 059 can reconcile them. Never infer an absent source EP balance.
- ADR 059 supersedes website-only Sheet archival: Raid Archive outcomes adopt
  archive state, loot and participant Sheet EP atomically with compensating ledger
  entries and audit. No second completion reward or new recurrence chain. Proven
  setup/series identity can bind active raids; first archive adoption may accept
  no-ID loot. Original import mappings can be referenced by the authorized copy
  through an immutable scope relation; the Original Sheet is never fetched or
  written by continuous sync. Concurrent website EP and ambiguous loot conflict.
- The owner requested continuous synchronization instead of a one-off import,
  including the current raid setup. The first delivery is conservative:
  historical tabs are continuously previewed/merged with existing provenance,
  the first numbered setup block can add known characters to the single active
  raid, and the application publishes read-only Website-prefixed mirror tabs.
  Source corrections/removals and Website-mirror edits are conflicts rather
  than automatic last-writer-wins changes. A fully editable two-way master
  remains explicitly unsupported until correction/ownership rules are decided.
- Sync is opt-in via server-side Google service-account credentials. It must
  never export authentication secrets or overwrite existing source tabs.
- Source tabs: `Raid Archive`, `Loot Archive`, `Charaktere`, `Punkte`.
  Current setup additionally reads `ICC25 Raidsetup` A–F (ADR 029): the first numbered
  roster block, with explicit administrator-supplied schedule and leader. One source/sheet
  bootstrap creates a PLANNED raid and series; repeat imports preserve that setup.
  Sheet ID `1N5EgBixB1RNxZagK5d0ie_DMTcYmznpvW5UzuyEWgRU`; fetched as CSV via
  `https://docs.google.com/spreadsheets/d/<id>/gviz/tq?tqx=out:csv&sheet=<escaped-name>`.
- Sheet date format is German `DD.MM.YYYY`; raid IDs look like `ICC25-01`.
- `Loot Archive`'s `Spieler` column contains character names, not player names (ADR 035).
  Resolve the character's owner and persist the source character name on the loot award.
  Loot alone never fabricates attendance; existing imports are not silently rewritten.
- **Never overwrite existing data.** Import is dry-run first, merge only when explicitly requested,
  and always returns a report (ADR 008).
- Imports are de-duplicated by `raw_hash`: an identical source is skipped and the previous report is
  returned instead of re-processing.
- Parse problems are reported as warnings, not silent data loss.
- The sheet's `Priority` is recorded as the award method: `Main` → `MAIN`, `Second`/`Token` →
  `SECOND`, `Enhanced` → `ENHANCED`, `BoE` → `BOE`. Unknown or empty values become `SECOND` plus a
  warning, never main-spec loot (ADR 028).
- An imported armor token never blocks +10 EP, whatever priority the sheet recorded; the token
  ledger still follows the item. An imported `ENHANCED` award counts as main-spec loot but costs 0
  EP — no historical enhanced charge is ever recreated.
- An imported loot boss is derived from the bundled catalog only when the item belongs to exactly one
  boss; ambiguous or uncatalogued items stay unknown and are reported. Item names are never
  rewritten.
- The sheet raid key is the catalog context: `ICC25-01` → instance `631`, `raid_size` `25`, applied
  only to raids this import creates.
- Rising-Gods enrichment during import uses exact per-character lookup. This remains best-effort
  metadata enrichment and never blocks historical imports (ADR 044).

### Core hardening decisions (ADR 017-020)
- Only one matching active item lock may exist per raid. Competing locks are rejected;
  one player may still lock multiple different items. Item IDs are authoritative when both
  records have IDs; otherwise matching uses the trimmed item name.
- Attendance and loot corrections use explicit actor + transactional before/after audit snapshots.
  Planned, open, and completed raids can be corrected, including archived completed raids.
  Cancelled and inconsistently archived active raids remain blocked. Archived corrections and
  additions require a nonblank reason; unarchived writes do not (ADR 032).
- Completed corrections append only the before/after EP eligibility difference. This preserves
  imported opening balances and never recreates historical raid rewards. Token visibility follows
  the corrected loot award; consumed locks are not implicitly revived.
- Incremental loot imports use an optional permanent, unique `Import ID` source column.
  Edited IDs, duplicate IDs, and ambiguous no-ID changes are conflicts, not overwrites.
  A conflict rejects the entire merge with a report and no domain writes.
- Import raid identity is scoped to source and sheet provenance. New rows in existing
  imported raids require audited correction consequences and an explicit reason when archived; name
  collisions without provenance and additions to cancelled raids are conflicts.
  ADR 029 permits ADMIN import-only additions to provenance-mapped archived COMPLETED
  raids, with actor/reason audit and append-only EP/token differences. ADR 032 also permits ordinary
  archived corrections by raid staff with a reason. Imported historical raids are archived; archival
  classification of older imports never re-awards EP or creates recurrence successors.
- An error-free server-side preview must exist for the source, sheet, and raw hash before a
  new merge. Preview metadata may be persisted; previews never write domain records.
- Production requires explicit nondefault bootstrap credentials. Configuration, migrations,
  and bootstrap failures stop startup. SQLite foreign keys are enabled on every connection.

### Loot catalog is bundled and read-only (ADR 027)
- `scripts/data/catalog-wotlk335.json` is the sole catalog authority. It is embedded with
  `go:embed` and mirrored into the catalog tables at startup. There is no upload, preview, or
  merge; updating the catalog means committing a new file. An invalid source or a failed mirror
  stops startup.
- Catalog reads are served from the embedded source, never from the mirror. The mirror exists to
  keep historical foreign keys valid; rows are retained, never deleted.
- Source paths are normalized server-side into `NORMAL`, `HEROIC`, and `HARDMODE`. Relationships
  sharing boss, item, raid size, and mode are merged; raw `difficulty`/`loot_mode` pairs survive
  as `source_variants`, and unproven conditions are marked `conditional`. Loot masks never imply
  a mode.
- New loot must reference a bundled boss/item pair for the raid's persisted
  `catalog_instance_id` and `raid_size`; names are derived server-side. Missing context is
  `catalog_context_required`, an unproven pair is `catalog_item_invalid`.
- Historical free-text and imported loot is never rewritten to match the current catalog.

---

## Authentication and permissions (ADR 010)

- Local accounts. `username` + bcrypt `password_hash` live on the player row.
- Login returns an opaque random session token, stored sha256-hashed, in the `sessions` table and
  sent as an `HttpOnly`, `SameSite=Lax` cookie named `raid_session`. `Secure` when
  `COOKIE_SECURE=true` or `ENV=production`. Sessions last 30 days and are validated per request;
  unknown or expired cookies mean *unauthenticated* (not error).
- The legacy `X-Actor-ID` header is **removed** and must not come back.
- Roles, ascending: `GUEST` < `MEMBER` < `RAIDASSISTENT` < `RAIDLEADER` < `ADMIN`, stored in
  `players.role`; higher roles include all powers of lower ones (`Role.AtLeast`).
- Matrix:
  - **Read** — any authenticated role: all `GET` endpoints.
  - **Config (`ADMIN`)** — create player; set account/credentials; create or update characters;
    guild ranks; raid groups; recurring raid series; list/import legacy data.
  - **Raid operations (`RAIDASSISTENT` and up, i.e. RA + RL)** — create raid; configure its
    series/start; cancel; archive/finalize; attendance; loot; create and close polls; create
    character and optional player together with attendance through raid quick-add (ADR 037).
  - **Self-service (`MEMBER` and up)** — create and cancel **own** item locks;
    vote on polls.
- The first admin is bootstrapped from `ADMIN_USERNAME` / `ADMIN_PASSWORD` at startup.
- Login MFA options are server-reported per challenge. The frontend shows only configured factors;
  WebAuthn/FIDO2 security keys are the default option when available and supported (ADR 047).
- A Rising-Gods profile has at most one pending, approved, or consumed registration request. Rejected
  requests may be submitted again; existing duplicate history is retained (ADR 055).

---

## API conventions

- All endpoints live under `/api/v1`. Version changes go in a new package, not in place.
- Success envelope: `{"data": ...}`. Errors: `{"error": {"code": "...", "message": "..."}}`.
- JSON is **snake_case**, on models and request DTOs alike. Request decoding uses
  `DisallowUnknownFields`, so DTOs must cover exactly the accepted fields.
- **List endpoints must always encode an empty JSON array, never `null`** — use
  `writeListJSON` / `nonNil` in `internal/httpapi/v1`. The frontend iterates these responses
  unguarded. Same for nested lists such as `transactions`. This is covered by
  `TestListEndpointsReturnEmptyArrays`; do not bypass it with a raw `WriteJSON` on a slice.
- Handlers resolve the actor from the session via the context, never from a request header.
- Timestamps are RFC3339 UTC strings; the client formats them locally.

---

## Frontend conventions

- Sessions travel in the `raid_session` cookie, so all API calls must use `credentials: 'include'`.
  In dev, Vite proxies `/api` to `http://localhost:8080` so the browser stays same-origin.
- Routing is a small hash router (`web/src/router.tsx`). Keep it dependency-free unless the owner
  approves a routing library.
- Authorization is enforced by the server; the UI may hide controls for convenience but must not be
  the only check.
- The compiled SPA is built by Vite into `internal/webui/dist` and embedded with `go:embed`, so the
  frontend must be built **before** the Go binary. `web/go.mod` exists only to keep `node_modules`
  out of the root Go module.
- User-facing text is German.
- The visible application version and changelog live in `web/src/release.ts`. Every user-facing
  change increments the patch version and adds a concise German changelog entry in the same change.
- Visual identity: "Bratwurst des Schicksals", dark premium ICC-inspired navy/black panels,
  icy blue/cyan/silver accents, original SVG/CSS crest and atmosphere, class-colored details,
  restrained frost borders/glow, readable typography, desktop sidebar and mobile companion
  navigation. No fake statistics, notifications, progression, or unsupported controls (ADR 026).

---

## Commands and configuration

| Purpose | Command |
| --- | --- |
| Backend (dev) | `go run ./cmd/server` |
| Frontend (dev) | `cd web && npm run dev` (proxy → `:8080`) |
| Frontend build | `cd web && npm run build` |
| Go gates | `gofmt -w cmd internal`, `go vet ./...`, `go test ./...` |
| Frontend gates | `npm run build`, `npm run lint` |
| Frontend regression tests | `cd web && npm test` |

Environment variables: `PORT` (default `8080`), `DATABASE_URL` (default
`file:raid.db?_pragma=journal_mode(WAL)`), `ENV`, `COOKIE_SECURE`, `ADMIN_USERNAME`,
`ADMIN_PASSWORD`.

Build order matters: `make build` runs the frontend build first, then `go build`.

---

## Toolchain notes

Prefer the documented repo commands and avoid machine-specific assumptions.

### Linux / NixOS

- Use normal POSIX shell command chaining (`cmd && next`) when commands depend on prior success.
- `make build` is the preferred full build when `make`, Go, Node, and npm are available in the
  active shell or development environment.
- If using a Nix shell/dev environment, enter it before running Go, npm, or Python catalog commands.

### Windows

- PowerShell 5.1 does not support `&&`; use `cmd; if ($?) { next }` for dependent commands.
- When writing UTF-8 files with PowerShell cmdlets, specify UTF-8 explicitly to preserve German text.
- If `make` is unavailable or resolves to an incompatible shell, run the underlying `npm` and `go`
  commands directly.

Build order matters: run the frontend build before the Go binary when producing an embedded SPA.
Verify generated or formatted output with the relevant build/test command instead of trusting
terminal rendering alone.

---

## Definition of done

A change is done when:

1. It satisfies a rule above, or the owner has explicitly changed a rule.
2. Business logic lives in the domain package; handlers stay thin.
3. It is covered by tests, including the permission boundary it touches.
4. `gofmt`, `go vet`, `go build ./...`, `go test ./...`, `npm run build`, `npm run lint` all pass.
5. Any behaviour change is reflected in the ADR set. New or changed decisions get a new ADR file
   that supersedes or amends the old decision; do not silently rewrite old project history.

---

## Known gaps — do not assume these exist

- No `DELETE` endpoint is exposed. Archive/expire flows only (ADR 007 remains the intent).
- `internal/export` is an empty directory; general data export is not implemented. The RaidSync
  addon exchange lives in `internal/addonsync` (ADR 045).
- The audit log is written but has no read endpoint yet.
