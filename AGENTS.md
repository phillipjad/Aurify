# AGENTS.md

Guidance for humans and AI agents working in this repository. Read this before
making changes.

## What Aurify is

A PWA that generates abstract playlist/album covers by analyzing a playlist's
audio features and lyric sentiment, then driving local LLM sidecars. See
[README.md](README.md) for the vision and [docs/adr/](docs/adr/) for the *why*
behind the structure. **This is a scaffold**: prefer extending the existing
shapes over inventing new ones.

## Repository map

- `backend/` — Go 1.26 API. Authoritative layout in
  [backend/README.md](backend/README.md).
- `frontend/` — React 19 PWA. Layout in [frontend/README.md](frontend/README.md).
- `docs/adr/` — Architecture Decision Records. Add one for any significant or
  hard-to-reverse decision.

## Architecture rules (do not break these)

**Backend — dependency direction points inward:**

- `internal/domain` imports nothing internal (pure types).
- `internal/app/ports` imports only `domain` (these are the interfaces the app
  depends on).
- `internal/app/{command,query}/*` depend on `ports` + `domain` only.
- `internal/platform/*` and `internal/storage/*` are adapters that *implement*
  `ports`. They must not be imported by the app layer.
- `cmd/api` is the **only** place that wires concrete adapters together.
- Lightweight CQRS: writes go through `command/`, reads through `query/`. One
  sub-package per use case; register it on the relevant `Bus`. No event store
  (see [ADR 0003](docs/adr/0003-lightweight-cqrs.md)).
- All HTTP/`fgrzl/mux` usage stays inside `internal/transport/http`.

**Frontend — mirror the CQRS split:**

- Reads (queries) live in `src/lib/api/queries.ts` (TanStack `useQuery`).
- Writes (mutations/commands) live in `src/lib/api/commands.ts` (`useMutation`).
- All network access goes through `src/lib/api/client.ts`.
- Routes are file-based under `src/routes/`; `routeTree.gen.ts` is generated.
- Wire types are **generated** from the backend OpenAPI spec, not hand-written:
  `src/lib/api/schema.ts` (via `pnpm gen:api`) is the source, and
  `src/lib/api/types.ts` derives from it. Don't edit field shapes by hand
  (see [ADR 0008](docs/adr/0008-openapi-contract.md)).

## Conventions

- **Go**: standard `gofmt`/`goimports` (local prefix
  `github.com/phillipjad/aurify/backend`). Wrap errors with `%w`. Map domain
  sentinel errors to HTTP in `transport/http/handlers/common.go`. Keep handlers
  thin — logic belongs in command/query handlers.
- **TypeScript**: ESLint flat config with **Prettier built in** via
  `eslint-plugin-prettier` (no semicolons, single quotes). Formatting is not a
  separate step — `pnpm lint` checks it (as the `prettier/prettier` rule) and
  `pnpm lint:fix` applies it. Import via the `@/` alias. Wire types come from the
  generated OpenAPI schema (see the frontend rule above) — never re-declare DTO
  shapes by hand.
- **API contract**: change a Go DTO/route, then regenerate both artifacts:
  `cd backend && go generate ./...` (writes `api/openapi.yaml`), then
  `cd frontend && pnpm gen:api` (writes `src/lib/api/schema.ts`). Both are
  committed.
- Stubs are marked with `SCAFFOLD:` / `TODO:` comments — search for these to find
  what's left to implement.

## Build / test / run

This project does **not** use Make. Backend builds go through `build.go`; all
other tasks use native `go` commands directly.

```bash
# backend — build (run from backend/)
go run build.go                   # version defaults to "dev"
VERSION=1.2.3 go run build.go     # explicit version (matches CI semver output)

# backend — check / test / format (run from backend/)
gofmt -w .
go vet ./...
go test ./...
go generate ./...                 # regenerate api/openapi.yaml from the routes

# frontend (run from frontend/)
pnpm install && pnpm gen:api && pnpm typecheck && pnpm lint && pnpm build
```

Run locally, two options:

- **Full stack in Docker:** `./start_container.sh -mb` (the `-m` creates the
  Mongo host data dir on first run; add `-d` to detach). See its `--help`.
- **Native dev loop:** `docker compose up -d mongo`, then
  `go run build.go && ./bin/aurify` (backend) and `pnpm dev` (frontend).

**Always** run `gofmt`/`go vet` on the backend and `pnpm typecheck` on the
frontend before considering a change done.

## Git hooks (lefthook)

[lefthook](https://lefthook.dev) runs lint + build checks on `pre-commit`,
**scoped by path**: backend commands fire only when `backend/**` is staged,
frontend commands only when `frontend/**` is staged (see `lefthook.yml`).

```bash
# install lefthook once (pick one): go / brew / npm
go install github.com/evilmartians/lefthook@latest
# then wire the hooks into .git/hooks (run once per clone):
lefthook install
```

The backend lint command needs `golangci-lint` on PATH
(<https://golangci-lint.run/welcome/install/>); it fails fast with a hint if
missing. Test scoping without committing: `lefthook run pre-commit --files <path>`.

## Gotchas

- `fgrzl/mux` is pre-1.0 and its docs aren't on pkg.go.dev — read its source /
  `examples/`. Path params use `{brace}` syntax. ([ADR 0007](docs/adr/0007-fgrzl-mux-web-framework.md))
- `mongo-driver/v2`: `mongo.Connect` takes **no** context; `primitive` is merged
  into `bson`. ([ADR 0004](docs/adr/0004-mongodb-storage.md))
- The Go module path is lowercase (`github.com/phillipjad/aurify/backend`); the
  GitHub repo is `Aurify`.
- Don't commit secrets — DSP credentials go in `backend/.env` (gitignored).
