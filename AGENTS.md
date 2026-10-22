# AGENTS.md — raid-website

Operating rules for AI agents and humans who change this repository. The project is specified
before it is changed: behaviour, design, architecture, and security live in the four spec files
below. This file adds only how to work; it defines no behaviour or architecture.

@REQUIREMENTS.md
@DESIGN.md
@ARCHITECTURE.md
@SECURITY.md
@CODING_RULES.md

## Sources of truth

| File | Owns |
| --- | --- |
| `REQUIREMENTS.md` | What the system does (`FR-*`, `NFR-*`, `OQ-*`) |
| `DESIGN.md` | Look, components, accessibility, rendering safety (`DQ-*`) |
| `ARCHITECTURE.md` | Components, data ownership, flows, trust boundaries (`DR-*`, `AQ-*`) |
| `SECURITY.md` | Secure-coding rules and security decisions (`SC-*`, `OD-*`) |
| `CODING_RULES.md` | Code conventions (German) |
| `REQUIREMENT_TEMPLATE.md` | Body of every leaf implementation issue |
| `docs/decisions/*.md` | Decision history (ADRs): why a rule exists |

- Each topic has exactly one owning file. Other files reference its IDs and never restate it.
- Where a spec entry says "not built", "not compliant", or "The code still …", the decision is made
  and the code is behind: change the code, not the spec. ("Owner 2026-10-22" only marks who
  decided; such a rule may already be in the code.)
- Code and spec disagree in a way the spec does not explain: stop and ask the owner.

## Working rules

1. **Do not guess. Ask.** If a rule, number, status, role, or workflow is not in the spec files,
   it is unknown. Ask the owner; never invent it.
2. **Spec first.** A change in behaviour, design, architecture, or security updates the owning
   spec file in the same change, with the owner's decision. Record new open items under the
   owning file's next free ID (`OQ-*`, `DQ-*`, `AQ-*`, `OD-*`). IDs are never reused.
3. **Decisions are history.** When the owner changes or rejects an earlier decision, add a new ADR
   with the next number that supersedes or amends the old one. Do not rewrite accepted ADRs, and do
   not enforce an outdated ADR against the owner's current answer (ADR 043).
4. **Backlog.** Work is tracked as GitHub issues. Every leaf implementation issue MUST follow
   `REQUIREMENT_TEMPLATE.md`. Epic, workstream, and decision (`REQ-DECISION-*`) issues use short,
   purpose-specific bodies that link the affected spec IDs and issues. One branch and one pull
   request per leaf issue, linked with `Closes #<number>`.
5. **New dependencies** (Go or npm) need the owner's approval first (DR-11).
6. **Never commit secrets** or production data (SC-033).

## Commands

| Purpose | Command |
| --- | --- |
| Backend (dev) | `go run ./cmd/server` |
| Frontend (dev) | `cd web && npm run dev` (Vite proxies `/api` to `:8080`) |
| Full build | `make build` (frontend first, then Go; the SPA is embedded) |
| Frontend build | `cd web && npm ci && npm run build` (writes `internal/webui/dist`) |
| Go gates | `gofmt -l cmd internal` (no output), `go vet ./...`, `golangci-lint run`, `go build ./...`, `go test ./...` |
| Frontend gates | `cd web && npm run lint && npm test` |
| End-to-end | `cd web && npx playwright install chromium && npm run e2e` |
| Guide screenshots | `cd web && npm run build && DOCS_TOUR=1 npx playwright test e2e/zz-docs-tour.spec.ts` |
| Sync addon bundle | `go run ./cmd/sync-addons <wow-addons checkout>` |

- Development defaults: `PORT=8080`, `DATABASE_URL=file:raid.db?_pragma=journal_mode(WAL)`.
  Production configuration is described in `deploy/README.md` and `SECURITY.md` SC-033.
- `web/go.mod` exists only to keep `node_modules` out of the root Go module.
- A gate that cannot run is reported, never skipped silently.

## Definition of done

A change is done when:

1. It implements its issue's acceptance criteria, and the owning spec files describe the result
   (resolved "not built" notes removed).
2. Tests cover the new behaviour, its permission boundary, and its failure paths
   (`CODING_RULES.md` §13).
3. All Go and frontend gates pass.
4. A changed decision has a new ADR.
5. A user-facing change bumps the patch version in `web/src/release.ts` and adds a short German
   changelog entry there.
6. A changed screen that `/anleitung` describes updates `web/src/pages/Guide.tsx` and regenerates
   its screenshots.
7. A new addon version is synced with `cmd/sync-addons`, and `web/src/pages/Addons.tsx` and
   `web/public/addons/` screenshots are updated in the same change. Screenshots never show account
   names, passwords, or the login screen.

## Toolchain notes

- **Linux / NixOS:** chain dependent commands with `&&`; enter the Nix dev shell first if you use
  one.
- **Windows:** PowerShell 5.1 has no `&&`; use `cmd; if ($?) { next }`. Write files as UTF-8
  explicitly so German text survives. Without a working `make`, run the `npm` and `go` commands
  directly.
- Check generated or formatted output with the build and test commands, not by reading terminal
  output alone.
