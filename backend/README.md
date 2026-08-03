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
    llm/{promptgen,imagegen}/# prompt + image model clients (ADR 0016)
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
- PostgreSQL 17 and Mailpit (`./start_container.sh -dm postgres mailpit` from the
  repo root)
- [`sqlc`](https://docs.sqlc.dev/en/latest/overview/install.html) on PATH — only
  needed to regenerate the `db` package after changing SQL

## Quick start

```bash
../scripts/dev-setup.sh      # generates backend/.env.dev (Mailpit + a signing key)
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
| GET    | `/api/v1/auth/{platform}/login`       | command | 302 to the provider's consent screen |
| GET    | `/api/v1/auth/{platform}/callback`    | command | links a DSP account, 302 back to `/playlists` |
| POST   | `/api/v1/covers`                      | command | analyze a playlist + generate a cover |
| GET    | `/api/v1/playlists?platform=`         | query   | list the user's playlists |
| GET    | `/api/v1/covers`                      | query   | list the user's covers |
| GET    | `/api/v1/covers/{id}`                 | query   | get one cover |

`{platform}` is one of `spotify`, `apple_music`, `youtube_music`.

## Authentication

Email/password and Sign in with Google, delivered by cookie. See
[ADR 0011](../docs/adr/0011-authentication-and-sessions.md) for the design and
[ADR 0012](../docs/adr/0012-account-lockout-policy.md) for the lockout policy.

- A **15-minute Ed25519 access token** is verified statelessly on every request,
  so an authenticated call costs no database round trip. The price is that a
  revoked session keeps working until its access token expires.
- A **256-bit opaque refresh token** (30-day idle, 90-day absolute) is stored as
  a SHA-256 digest and rotated on every use. Presenting an already-rotated token
  means the value leaked, so the whole session is revoked.
- `mux.UseAuthentication` supplies only the mounting point and the
  `AllowAnonymous()` bookkeeping; the validator, session store, rotation, CSRF
  and lockout are all in this repo.
- `currentUser` in `internal/transport/http/handlers/common.go` reads the
  verified principal. The `X-User-ID` header it replaced was a complete
  authentication bypass and must not come back.

Set `AURIFY_AUTH_SIGNING_KEY` in production — unset, the API generates an
ephemeral key, so sessions die on restart and two instances reject each other's
tokens. Google sign-in is off unless `AURIFY_GOOGLE_CLIENT_ID`/`_SECRET` are
set. See `.env.default`.

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
- Spotify and Apple Music providers are stubbed; YouTube Music is built.
- Prompt and image generation call OpenAI-compatible providers, defaulting to
  Ollama on the host and Cloudflare Workers AI. Each falls back to a deterministic
  placeholder when unconfigured: `AURIFY_PROMPTGEN_URL` empty for prompts,
  `AURIFY_IMAGEGEN_API_KEY` empty for images (the image URL and model have
  working defaults, so the key is the switch). See
  [ADR 0016](../docs/adr/0016-generation-providers.md).
- Module path is `github.com/phillipjad/aurify/backend`. The GitHub repo is
  `Aurify` (capitalized); Go module paths are lowercase by convention, so for
  `go get` to work via the proxy the repo should be referenced in lowercase.
