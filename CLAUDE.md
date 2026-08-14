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
- Things marked `SCAFFOLD:` / `TODO:` are deliberately unfinished (Spotify and
  Apple Music network calls). Grep for them.

## Before you finish

- Backend: `cd backend && gofmt -w . && go vet ./... && go build ./...`
  - Do **not** use Make. Build the binary with `go run build.go` (version defaults
    to `dev`; CI injects the real semver via `VERSION=x.y.z go run build.go`).
- Frontend: `cd frontend && vp check` (Oxfmt + Oxlint + tsgolint, all via Vite+)
- Add or update an ADR if you made a significant/hard-to-reverse decision.

## External libraries to be careful with

- `fgrzl/mux` (pre-1.0, docs not on pkg.go.dev — read its source/examples).
- `sqlc` generates the typed `db` package from SQL; after editing a migration or
  `internal/storage/postgres/query/*.sql`, run `cd backend && sqlc generate` (see
  [ADR 0010](docs/adr/0010-postgresql-storage.md)). Never edit the generated `db/`.
- Vite+ (`vp` CLI) is the **only** frontend toolchain — every task runs through it
  (`vp dev/check/test/build`); there is no plain `pnpm`/`vite`/`eslint` path.
