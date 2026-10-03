# Raid-Website

WotLK Raid management (Armor Tokens, Enhance Points, Loot, Attendance). Modular monolith: Go backend (Bun + SQLite) + React/TypeScript frontend.

## Quick Start (dev)

- Backend: `go run ./cmd/server` (after setup)
- Frontend: `cd web && npm run dev`

## Quality Gates

- `gofmt` / `go vet` / `golangci-lint` / `go build ./...` / `go test ./...`
- TypeScript strict + ESLint

See ARCHITECTURE.md and docs/decisions/ for details.
