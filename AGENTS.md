# AI Working Agreement — raid-website

Single entry point for AI agents (and humans) working in this repository. It records the
requirements and constraints the project owner defined **before** implementation started, so that
future agents do not re-litigate them or silently change them.

**If a rule is not listed here, you do not know it. Do not invent it — ask.**

Authoritative sources, in order of precedence:

1. This file (owner-level requirements and constraints)
2. `docs/decisions/*.md` (one accepted decision per file, ADR 001-026)
3. `ARCHITECTURE.md` (module map and principles)
4. `CODING_RULES.md` (defines the coding rules)
5. `README.md` (commands)

---

## 1. What this project is

A WoW WotLK guild raid-management website: raids, attendance, loot, Enhance Points, Armor Tokens,
polls, roster configuration, and a one-off import of a legacy Google Sheet.

Architecture is a **modular monolith** (ADR 001): Go backend + SQLite via Bun (ADR 002), React +
TypeScript frontend built with Vite. One binary serves both the JSON API and the compiled SPA.

---

## 2. Non-negotiable engineering rules

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
4. **Append-only ledgers.** Enhance Points, Armor Tokens, and the audit log are append-only.
   Balances are computed by summing transactions. Never `UPDATE` or delete history; a correction is
   a new, compensating entry (ADR 004, ADR 005).
5. **Never silently overwrite.** Legacy import and any bulk write must produce a report and must not
   clobber existing data (ADR 008).
6. **Archive by default.** History-affecting records are archived (`archived_at`), never deleted.
   A hard delete must be explicitly requested by a human and is Admin-only (ADR 007).
   For raids, archiving means final processing, not removal or hiding (ADR 021).
7. **Edits are audited.** Changes to history-affecting records (loot, attendance, character renames)
   require actor + reason and write an audit log entry (ADR 006). Numeric corrections prefer new
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

## 3. Stack (fixed — do not substitute)

- Go 1.26, `net/http` with the Go 1.22+ method-pattern `ServeMux` (`mux.HandleFunc("GET /api/v1/...")`).
- Bun ORM v1.3 with the SQLite dialect (`sqlitedialect` + `sqliteshim` + `modernc.org/sqlite`).
- SQL migrations as numbered `.up.sql` / `.down.sql` files embedded via `go:embed`
  (filename regex: `^(\d{1,14})_([0-9a-z_\-]+)\.`).
- Frontend: React 18 + TypeScript (strict) + Vite. No state-management or HTTP library was added, so
  the client is a small hand-rolled fetch wrapper plus a hash router. **Adding a dependency is a
  decision for the owner — ask first.**
- Password hashing: `golang.org/x/crypto/bcrypt`.

---

## 4. Domain rules the owner confirmed

Every item here was explicitly decided by the owner. Do not "improve" on them.

### 4.1 Players and characters
- A player may own multiple characters. Each character has a class, a main spec, and a second spec.
- `is_main` marks the character the player currently raids on as main — it is a preference flag, not
  a uniqueness constraint.
- Character names are mutable because players rename in game. Therefore **no DB-level global
  `UNIQUE(name)`**; records are addressed by stable IDs and renames are recorded in name history
  (ADR 013).

### 4.2 Raids
- Lifecycle: `PLANNED` → `OPEN` → `COMPLETED`, with cancellation as `CANCELLED`.
  `Archivieren` is the single final action: it sets completed and archived timestamps together.
  There is no separate completion action (ADR 021 supersedes the former raid archive semantics).
- Archiving finalizes roster/loot processing, awards EP, expires unused locks, and schedules the
  next raid atomically. Repeated requests must neither award twice nor create duplicate successors.
- Archived raids remain visible as history, with read-only roster and loot.
- Each recurring raid series explicitly configures name, leader, weekday(s), week interval,
  local start time, and IANA timezone. No guild schedule is seeded or hard-coded.
- Recurrence weeks are anchored to the current raid's local Monday week. Archiving chooses the
  next future occurrence, skipping elapsed dates, and preserves local wall time across DST.
  Ambiguous/nonexistent local times fail explicitly rather than silently shifting the schedule.
- A preplanned eligible next occurrence is reused without replacing its data. A conflicting
  finalized/cancelled occurrence or another predecessor is reported, not overwritten.
- Existing stored completed raids are historical: archival never recreates their EP or starts
  a new recurrence chain. Existing active raids can be assigned a series and scheduled start.

### 4.3 Enhance Points (EP) — the core rule
- **+10 EP** for a player who received **no main-spec loot** in that raid.
- Awarded **only by final archiving**, which moves the raid to `COMPLETED` (ADR 021).
- **No EP at all if the raid is `CANCELLED`** (ADR 015).
- One award per (player, raid) — idempotent.
- Balance is `SUM(amount)` over the append-only `enhance_point_transactions` (ADR 004).

### 4.4 Recorded enhanced loot (ADR 023-024)
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
  All corrections are actor/reason audited and append compensating transactions atomically.
- Existing ledger history is retained, not recalculated under the revised rules.

### 4.5 Item locks
- A lock requires `balance >= 150` but **costs 0 EP** (ADR 012).
- A player may hold **multiple** simultaneous locks.
- If the locked item drops in that raid, it is auto-awarded to the locker **as a main-spec award**.
- If it does not drop, the lock **expires at raid completion** with no loss (ADR 012).

### 4.6 Attendance
- A **substitute** is a normal player; substitute and regular participants are treated identically.
- **Bench** participants follow the **same EP rule** as raiders: they earn +10 if they got no
  main-spec loot (ADR 016).
- The raid roster is editable until archival. It stores the actual character name, role,
  class, and spec played in that raid, independently of later live-character changes (ADR 022).
- Archive rejects an incomplete active roster. Legacy unknown snapshots are not fabricated;
  new sheet imports preserve Raid Archive's own character name, role, class, and spec.

### 4.7 Armor Tokens
- Tokens are derived from awarding the three specific loot items
  `Weiheabzeichen des Beschützers`, `Weiheabzeichen des Bezwingers`,
  `Weiheabzeichen des Eroberers` (ADR 014).
- Each such award appends a token transaction; the token ledger is append-only (ADR 005).
- The legacy sheet's "Tokens gesammelt" total is **reconciliation-only** and must never be written
  over derived data.

### 4.8 Loot, attendance, and character corrections
- Corrections are allowed but must be auditable: actor + reason + before/after snapshot
  (ADR 006).

### 4.9 Roster configuration
- Guild ranks and raid groups are configurable rows in the database, not hard-coded constants
  (ADR 009).

### 4.10 Legacy Google Sheet import
- Source tabs: `Raid Archive`, `Loot Archive`, `Charaktere`, `Punkte`.
  Sheet ID `1N5EgBixB1RNxZagK5d0ie_DMTcYmznpvW5UzuyEWgRU`; fetched as CSV via
  `https://docs.google.com/spreadsheets/d/<id>/gviz/tq?tqx=out:csv&sheet=<escaped-name>`.
- Sheet date format is German `DD.MM.YYYY`; raid IDs look like `ICC25-01`.
- **Never overwrite existing data.** Import is dry-run first, merge only when explicitly requested,
  and always returns a report (ADR 008).
- Imports are de-duplicated by `raw_hash`: an identical source is skipped and the previous report is
  returned instead of re-processing.
- Parse problems are reported as warnings, not silent data loss.

### 4.11 Core hardening decisions (ADR 017-020)
- Only one matching active item lock may exist per raid. Competing locks are rejected;
  one player may still lock multiple different items. Item IDs are authoritative when both
  records have IDs; otherwise matching uses the trimmed item name.
- Attendance and loot corrections use explicit actor + nonblank reason + transactional
  before/after audit snapshots. Corrections are available for planned, open, and completed
  raids, not cancelled or archived raids. Completed-raid additions also require a reason.
- Completed corrections append only the before/after EP eligibility difference. This preserves
  imported opening balances and never recreates historical raid rewards. Token corrections
  append compensating entries; consumed locks are not implicitly revived.
- Incremental loot imports use an optional permanent, unique `Import ID` source column.
  Edited IDs, duplicate IDs, and ambiguous no-ID changes are conflicts, not overwrites.
  A conflict rejects the entire merge with a report and no domain writes.
- Import raid identity is scoped to source and sheet provenance. New rows in existing
  imported raids require an explicit reason and audited correction consequences; name
  collisions without provenance and additions to cancelled/archived raids are conflicts.
- An error-free server-side preview must exist for the source, sheet, and raw hash before a
  new merge. Preview metadata may be persisted; previews never write domain records.
- Production requires explicit nondefault bootstrap credentials. Configuration, migrations,
  and bootstrap failures stop startup. SQLite foreign keys are enabled on every connection.

---

## 5. Authentication and permissions (ADR 010)

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
    series/start; cancel; archive/finalize; attendance; loot; create and close polls.
  - **Self-service (`MEMBER` and up)** — create and cancel **own** item locks;
    vote on polls.
- The first admin is bootstrapped from `ADMIN_USERNAME` / `ADMIN_PASSWORD` at startup.

---

## 6. API conventions

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

## 7. Frontend conventions

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
- Visual identity: "Bratwurst des Schicksals", dark premium ICC-inspired navy/black panels,
  icy blue/cyan/silver accents, original SVG/CSS crest and atmosphere, class-colored details,
  restrained frost borders/glow, readable typography, desktop sidebar and mobile companion
  navigation. No fake statistics, notifications, progression, or unsupported controls (ADR 026).

---

## 8. Commands and configuration

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

## 9. Toolchain traps on the owner's machine (read before running commands)

These cost real time to rediscover:

- The shell is **Windows PowerShell 5.1**, not bash. No `&&` chaining — use `;` and
  `if ($?) { ... }`. Heredocs (`<< EOF`) do not work; use
  `Set-Content -LiteralPath ... -Value @'...'@`.
- **`Set-Content` must be given `-Encoding UTF8`**, otherwise German umlauts are destroyed. This
  already corrupted ADRs 005 and 014 once (now repaired). Go rejects invalid UTF-8 sources, which is
  how the corruption would show up in code.
- `rg` and `sqlite3` are not installed. Use the editor's search tool or `Select-String`.
- `make` may not be on `PATH` (an MSYS2 copy exists at `C:\msys64\usr\bin\make.exe`), and that copy's
  bash cannot resolve `npm` under `C:\Program Files`. If `make` misbehaves, run the underlying
  `npm` / `go` commands directly.
- **Tool output is unreliable for formatting.** File contents shown back after a write can appear
  duplicated, reordered, or mangled. Treat that as a display artifact only. Verify with a read of the
  exact line range, or better, with `go build` / `tsc`, which are authoritative.
- The console code page renders `ü` and other non-ASCII characters as `?` or `�`. Confirm the real
  bytes (e.g. compare character code points) before "fixing" an encoding problem that isn't one.

---

## 10. Definition of done

A change is done when:

1. It satisfies a rule above, or the owner has explicitly changed a rule.
2. Business logic lives in the domain package; handlers stay thin.
3. It is covered by tests, including the permission boundary it touches.
4. `gofmt`, `go vet`, `go build ./...`, `go test ./...`, `npm run build`, `npm run lint` all pass.
5. Any behaviour change is reflected in the ADR set — new decisions get a new ADR file, they do not
   silently rewrite an old one.

---

## 11. Known gaps — do not assume these exist

- No `DELETE` endpoint is exposed. Archive/expire flows only (ADR 007 remains the intent).
- `internal/export` is an empty directory; exporting is not implemented.
- The audit log is written but has no read endpoint yet.
