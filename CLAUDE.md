# CLAUDE.md

This file orients Claude Code (and other AI agents) in the Aurify repository.

## Start here

The full contributor guide — architecture rules, conventions, build/test
commands, and gotchas — lives in **[AGENTS.md](AGENTS.md)**. Read it first; it is
the source of truth and applies to you.

Decision rationale is in **[docs/adr/](docs/adr/)**. The product vision and a
quick start are in **[README.md](README.md)**.

## TL;DR for making changes

- This is a **scaffold**. Extend existing patterns; don't restructure without an
  ADR.
- **Backend** (`backend/`): dependencies point inward
  (`domain` ← `ports` ← `app` ← adapters; `cmd/api` wires it all). Writes go in
  `internal/app/command/`, reads in `internal/app/query/` (lightweight CQRS).
  Keep all `fgrzl/mux` code in `internal/transport/http`.
- **Frontend** (`frontend/`): reads in `src/lib/api/queries.ts`, writes in
  `src/lib/api/commands.ts`, all fetches via `src/lib/api/client.ts`.
- Things marked `SCAFFOLD:` / `TODO:` are deliberately unfinished (DSP network
  calls, LLM sidecars, auth). Grep for them.

## Before you finish

- Backend: `cd backend && gofmt -w . && go vet ./... && go build ./...`
  - Do **not** use Make. Build the binary with `go run build.go` (version defaults
    to `dev`; CI injects the real semver via `VERSION=x.y.z go run build.go`).
- Frontend: `cd frontend && pnpm typecheck` (and `pnpm lint`)
- Add or update an ADR if you made a significant/hard-to-reverse decision.

## External libraries to be careful with

- `fgrzl/mux` (pre-1.0, docs not on pkg.go.dev — read its source/examples).
- `mongo-driver/v2` (API differs from v1; see [ADR 0004](docs/adr/0004-mongodb-storage.md)).
- Vite+ (`vp` CLI) is the frontend toolchain; plain `pnpm`/`vite` also work.
