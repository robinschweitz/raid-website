# Raid-Website

Raid management for the WotLK guild **Bratwurst des Schicksals**: raid planning and signups,
rosters, loot, Enhance Points, item locks, armor tokens, wishlists, polls, and the Shadowmourne
questline. One Go binary (Bun + SQLite) serves the JSON API and the embedded React/TypeScript
frontend.

## Specification

The project is specified before it is changed. Read these first:

| File | Contents |
| --- | --- |
| [`REQUIREMENTS.md`](REQUIREMENTS.md) | What the system does |
| [`DESIGN.md`](DESIGN.md) | Look, components, accessibility |
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | How it is built |
| [`SECURITY.md`](SECURITY.md) | Security rules |
| [`AGENTS.md`](AGENTS.md) | How to work on the code: rules, commands, definition of done |
| [`docs/decisions/`](docs/decisions/) | Decision history (ADRs) |

## Quick start (development)

```sh
go run ./cmd/server        # backend on :8080
cd web && npm run dev      # frontend, proxies /api to :8080
```

## Build

`make build` builds the frontend into `internal/webui/dist` and then the Go binary, which embeds
it. All gates and other commands are listed in `AGENTS.md`.

## Deployment

Production setup, backups, and recovery: [`deploy/README.md`](deploy/README.md).
