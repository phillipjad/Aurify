# Aurify

> Cover art that captures the vibe of your playlist.

Users across music platforms have cherished playlists but rarely a cover that
captures their feel. **Aurify** fills that gap: log into your DSP (Spotify, Apple
Music, YouTube Music), pick a playlist, and Aurify analyzes every track — its
audio features *and* the sentiment of its lyrics — then generates an abstract
cover from a color palette weighted to how the music feels.

> **Status: scaffold.** This repository is a structured skeleton. The shapes,
> interfaces, and wiring are real and the backend builds; authentication,
> storage, and deployment are implemented, while the external integrations (DSP
> network calls, prompt and image generation) are intentionally stubbed. See the
> per-component `README`s and `docs/adr/` for what is real vs. pending.

## How it works

```
DSP login ─▶ ingest tracks ─▶ per track: ─▶ aggregate + weight ─▶ prompt gen ─▶ image gen ─▶ cover
 (OAuth)     (normalized      audio features    into a color        (hosted       (hosted       (stored
             model)           + lyric sentiment palette              text API)     image API)    in Postgres)
                              via lrclib.net    [0,1] weights
```

Example of the weighting: a playlist with a mean liveness of `0.78` contributes
`0.78` (pre-normalization) to the *energetic* color; low valence plus negative
lyric polarity pushes weight toward the *melancholic* color. See
[`backend/internal/analysis/weights.go`](backend/internal/analysis/weights.go).

## Stack

| Layer | Choice |
|-------|--------|
| Backend | Go 1.26, [fgrzl/mux](https://github.com/fgrzl/mux), [pgx/v5](https://github.com/jackc/pgx) + [sqlc](https://sqlc.dev) |
| Storage | PostgreSQL 17 (relational + JSONB; see [ADR 0010](docs/adr/0010-postgresql-storage.md)) |
| Frontend | TypeScript 6, React 19, [Vite+](https://viteplus.dev/), [TanStack Query](https://tanstack.com/query) + [Router](https://tanstack.com/router), [Tailwind v4](https://tailwindcss.com), [shadcn/ui](https://ui.shadcn.com) |
| Auth | Email/password + Sign in with Google; Ed25519 access tokens, rotating refresh cookies (see [ADR 0011](docs/adr/0011-authentication-and-sessions.md)) |
| Architecture | Lightweight CQRS across backend & frontend (see [ADR 0003](docs/adr/0003-lightweight-cqrs.md)) |
| Deployment | One container on Cloud Run, secrets in Secret Manager, Postgres on Neon, all via OpenTofu (see [ADR 0013](docs/adr/0013-secrets-and-deployment.md)) |

## Repository layout

```
aurify/
├── backend/            # Go API (CQRS app layer, hexagonal ports/adapters)
├── frontend/           # React 19 PWA (Vite+, TanStack, Tailwind)
├── deploy/             # OpenTofu: app/ (Cloud Run + secrets) and database/ (Neon)
├── docs/adr/           # Architecture Decision Records
├── docs/environment-variables.md  # every setting the API reads
├── scripts/            # dev-setup.sh — generates the local backend/.env.dev
├── docker-compose.yml  # PostgreSQL + Mailpit (local SMTP capture)
├── start_container.sh  # runs the whole stack in Docker
├── AGENTS.md           # contributor & AI-agent guide
└── CLAUDE.md           # pointer for Claude Code
```

See [`backend/README.md`](backend/README.md),
[`frontend/README.md`](frontend/README.md), and [`deploy/README.md`](deploy/README.md)
for component details.

## Quick start

**First, once per clone** — the API requires a signing key and a mail relay in
every environment, so generate the local config:

```bash
./scripts/dev-setup.sh         # writes the gitignored backend/.env.dev
```

**Option A — full stack in Docker** (one command):

```bash
./start_container.sh -mb       # -m creates the Postgres data dir, -b builds images
                               # add -d to run detached; see ./start_container.sh --help
```

**Option B — native dev loop** (hot reload on the frontend):

```bash
# 1. Storage + mail (schema is created/migrated automatically on API startup)
./start_container.sh -dm postgres mailpit

# 2. API  (http://localhost:8080)
cd backend && go mod tidy && go run build.go && ./bin/aurify

# 3. PWA  (http://localhost:5173, proxies /api -> :8080)
cd frontend && vp install && vp dev
```

Verification and password-reset mail is captured by Mailpit; read it at
<http://localhost:8025>. Google sign-in stays disabled (its routes answer `501`)
until you supply a client ID and secret.

Before committing, install the git hooks once: `lefthook install` (see
[AGENTS.md](AGENTS.md#git-hooks-lefthook)).

Toolchain versions: Go 1.26.3 · Node 24.16.0 · pnpm 11.5.0 · PostgreSQL 17.

## What's stubbed (and where to look)

- **DSP integrations** — interfaces + registry are real; network calls return
  "not implemented" ([`backend/internal/platform/dsp`](backend/internal/platform/dsp), [ADR 0005](docs/adr/0005-dsp-provider-abstraction.md)).
- **Prompt + image generation** — built; placeholders when unconfigured
  ([`backend/internal/platform/llm`](backend/internal/platform/llm),
  [ADR 0016](docs/adr/0016-generation-providers.md)). Covers still look alike
  until the analysis improves ([#68](https://github.com/phillipjad/Aurify/issues/68)).
- **Admin tooling for lockouts** — a permanently blocked `(IP, address)` pair
  can currently only be cleared by an operator deleting the row
  ([ADR 0012](docs/adr/0012-account-lockout-policy.md)). Authentication itself
  is built: email/password and Sign in with Google, with rotating refresh
  tokens ([ADR 0011](docs/adr/0011-authentication-and-sessions.md)).
- **Lyrics + sentiment** — the lrclib client is real; sentiment uses a naive
  lexicon placeholder.

## Deploying

A single container on Cloud Run, secrets in Secret Manager, PostgreSQL on Neon,
all defined in OpenTofu under [`deploy/`](deploy) and applied by the `Deploy`
GitHub Actions workflow. The bootstrap steps are in
[`deploy/README.md`](deploy/README.md).
