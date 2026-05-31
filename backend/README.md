# Aurify — Backend (Go API)

The Aurify HTTP API. Go 1.26, [`fgrzl/mux`](https://github.com/fgrzl/mux) for
routing, [`mongo-driver/v2`](https://github.com/mongodb/mongo-go-driver) for
storage. The application layer is organized around **lightweight CQRS** (see
[`../docs/adr/0003-lightweight-cqrs.md`](../docs/adr/0003-lightweight-cqrs.md)).

## Layout

```
cmd/api/                     # entrypoint: wires adapters -> app -> mux router
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
  storage/mongo/             # MongoDB repositories (implement ports)
  transport/http/            # mux router, handlers, DTOs
```

The dependency rule points inward: `domain` imports nothing internal; `ports`
imports only `domain`; `app/*` depends on `ports` + `domain`; `platform/*` and
`storage/*` implement `ports`; `cmd/api` is the only place that knows the
concrete adapters.

## Prerequisites

- Go 1.26.3
- MongoDB 8.0 (e.g. `docker compose up mongo` from the repo root)

## Quick start

```bash
cp .env.example .env        # optional; sane defaults are built in
make tidy                   # resolve dependencies
make run                    # starts on :8080
```

Probes: `GET /livez`, `GET /readyz` (readiness pings MongoDB).

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

## Make targets

`make help` lists them: `tidy`, `build`, `run`, `test`, `lint`, `fmt`, `vet`.

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
