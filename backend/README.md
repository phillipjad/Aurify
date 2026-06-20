# Aurify — Backend (Go API)

The Aurify HTTP API. Go 1.26, [`fgrzl/mux`](https://github.com/fgrzl/mux) for
routing, PostgreSQL via [`pgx/v5`](https://github.com/jackc/pgx) +
[`sqlc`](https://sqlc.dev)-generated queries for storage (see
[`../docs/adr/0010-postgresql-storage.md`](../docs/adr/0010-postgresql-storage.md)).
The application layer is organized around **lightweight CQRS** (see
[`../docs/adr/0003-lightweight-cqrs.md`](../docs/adr/0003-lightweight-cqrs.md)).

## Layout

```
cmd/api/                     # entrypoint: wires adapters -> app -> mux router
cmd/openapi/                 # `go generate` target: emits api/openapi.yaml
api/openapi.yaml             # generated OpenAPI spec (the API contract)
internal/
  config/                    # env-based configuration
  domain/                    # pure domain types (no infra imports)
  app/
    ports/                   # interfaces the app depends on (hexagonal ports)
    command/                 # WRITE side — one sub-package per use case
      connectdsp/
      generatecover/
    query/                   # READ side — one sub-package per use case
      listplaylists/
      getcover/
      listcovers/
  analysis/                  # feature aggregation + color-weighting engine
  platform/                  # outbound adapters (implement ports)
    dsp/{spotify,applemusic,youtubemusic}/
    lyrics/lrclib/           # lrclib.net client (implemented)
    nlp/                     # lyric sentiment (naive lexicon placeholder)
    llm/{promptgen,imagegen}/# local LLM sidecar clients (stubbed — ADR 0006)
  storage/postgres/          # PostgreSQL repositories (implement ports)
    migrations/              # goose SQL migrations (embedded, applied on startup)
    query/                   # sqlc query sources
    db/                      # sqlc-generated typed queries (do not edit)
  transport/http/            # mux router, handlers, DTOs
```

The dependency rule points inward: `domain` imports nothing internal; `ports`
imports only `domain`; `app/*` depends on `ports` + `domain`; `platform/*` and
`storage/*` implement `ports`; `cmd/api` is the only place that knows the
concrete adapters.

## Prerequisites

- Go 1.26.3
- PostgreSQL 17 (e.g. `docker compose up postgres` from the repo root)
- [`sqlc`](https://docs.sqlc.dev/en/latest/overview/install.html) on PATH — only
  needed to regenerate the `db` package after changing SQL

## Quick start

```bash
cp .env.example .env        # optional; sane defaults are built in
go mod tidy                 # resolve dependencies
go run build.go             # compile -> bin/aurify (version stamped; defaults to "dev")
./bin/aurify                # starts on :8080
```

For a quick iteration loop you can also `go run ./cmd/api` — but note the binary
refuses to start without a stamped version, so pass one:
`go run -ldflags="-X main.version=dev" ./cmd/api`.

Probes: `GET /livez`, `GET /readyz` (readiness pings PostgreSQL).

## API (v1)

| Method | Path                                  | Side    | Notes |
|--------|---------------------------------------|---------|-------|
| GET    | `/api/v1/auth/{platform}/login`       | command | returns provider OAuth URL |
| GET    | `/api/v1/auth/{platform}/callback`    | command | links a DSP account |
| POST   | `/api/v1/covers`                      | command | analyze a playlist + generate a cover |
| GET    | `/api/v1/playlists?platform=`         | query   | list the user's playlists |
| GET    | `/api/v1/covers`                      | query   | list the user's covers |
| GET    | `/api/v1/covers/{id}`                 | query   | get one cover |

`{platform}` is one of `spotify`, `apple_music`, `youtube_music`.

## Authentication (scaffold note)

Session/JWT auth is **not** wired yet. `fgrzl/mux` ships
`mux.UseAuthentication(...)` which populates `c.User()`; until that is
configured, handlers read the caller's id from the `X-User-ID` request header.
Replace `currentUser` in `internal/transport/http/handlers/common.go` when real
auth lands.

## Common tasks

This project does **not** use Make. The release build goes through `build.go`
(so the version stamp is consistent with CI); everything else is plain `go`:

| Task | Command |
|------|---------|
| Build binary | `go run build.go` (or `VERSION=1.2.3 go run build.go`) |
| Format | `gofmt -w .` |
| Vet | `go vet ./...` |
| Test | `go test ./...` |
| Lint | `golangci-lint run ./...` |
| Gen OpenAPI | `go generate ./...` (writes `api/openapi.yaml`) |

### OpenAPI spec

`cmd/openapi` builds the real router and emits `api/openapi.yaml` — the single
source of truth for the HTTP contract. The frontend generates its TypeScript
types from it (`pnpm gen:api`), so the wire format can't drift. Regenerate after
changing any DTO or route, and keep the route `With*` decorations accurate. See
[ADR 0008](../docs/adr/0008-openapi-contract.md).

## Notes / TODO

- Cover generation runs **synchronously** in the request; move it to a queue
  for production (the handler is structured to make this easy).
- DSP providers, the OAuth `state`/CSRF handling, and token encryption-at-rest
  are stubbed.
- The two LLM sidecars are not built; the clients fall back to deterministic
  placeholders when `AURIFY_PROMPTGEN_URL` / `AURIFY_IMAGEGEN_URL` are empty.
- Module path is `github.com/phillipjad/aurify/backend`. The GitHub repo is
  `Aurify` (capitalized); Go module paths are lowercase by convention, so for
  `go get` to work via the proxy the repo should be referenced in lowercase.
