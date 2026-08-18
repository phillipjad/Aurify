# Aurify

[![CI](https://github.com/phillipjad/Aurify/actions/workflows/ci.yml/badge.svg?branch=develop)](https://github.com/phillipjad/Aurify/actions/workflows/ci.yml)

> Cover art that captures the vibe of your playlist.

Users across music platforms have cherished playlists but rarely a cover that
captures their feel. **Aurify** fills that gap: log into your DSP (Spotify, Apple
Music, YouTube Music), pick a playlist, and Aurify analyzes every track — its
audio features *and* the sentiment of its lyrics — then generates an abstract
cover from a color palette weighted to how the music feels.

> **Status: working on YouTube Music; one provider deep, not three wide.**
> YouTube Music runs the whole path for real — OAuth, playlist and track ingest,
> analysis, palette, generation, and writing the finished image back as the
> playlist's artwork. Auth, sessions, storage, and deployment are implemented.
> Spotify and Apple Music implement the same `DSPProvider` port but their network
> calls are still stubs, and generation falls back to a placeholder image until
> provider credentials are supplied. See [What's stubbed](#whats-stubbed-and-where-to-look)
> and `docs/adr/` for the detail.

## How it works

```
DSP login ─▶ ingest tracks ─▶ per track: ─▶ aggregate + weight ─▶ prompt gen ─▶ image gen ─▶ cover
 (OAuth)     (normalized      audio features    into a color        (hosted       (hosted       (stored
             model)           + lyric sentiment palette              text API)     image API)    in Postgres)
                              via lrclib.net    [0,1] weights
```

Example of the weighting: a playlist with a mean energy of `0.78` contributes
`0.78` (pre-normalization) to the *energetic* color; a playlist mostly in minor
keys, with bleak lyrics, pushes weight toward the *melancholic* color. Every
dimension reads something a source actually measures
([ADR 0025](docs/adr/0025-palette-from-measured-signals.md)). See
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

# 2. API  (http://localhost:18080)
cd backend && go mod tidy && go run build.go && ./bin/aurify

# 3. PWA  (http://localhost:5173, proxies /api -> :18080)
cd frontend && vp install && vp dev
```

Verification and password-reset mail is captured by Mailpit; read it at
<http://localhost:8025>. Google sign-in stays disabled (its routes answer `501`)
until you supply a client ID and secret.

Before committing, install the git hooks once: `lefthook install` (see
[AGENTS.md](AGENTS.md#git-hooks-lefthook)).

Toolchain versions: Go 1.26.3 · Node 24.16.0 · pnpm 11.5.0 · PostgreSQL 17.

## What's stubbed (and where to look)

- **DSP integrations** — YouTube Music is implemented end to end: OAuth, playlist
  and track ingest, and setting the generated image as the playlist's artwork.
  Spotify and Apple Music implement the same port, but their network calls still
  return "not implemented"
  ([`backend/internal/platform/dsp`](backend/internal/platform/dsp), [ADR 0005](docs/adr/0005-dsp-provider-abstraction.md)).
- **Prompt + image generation** — built; placeholders when unconfigured
  ([`backend/internal/platform/llm`](backend/internal/platform/llm),
  [ADR 0016](docs/adr/0016-generation-providers.md)). Covers still look alike
  until the analysis improves ([#68](https://github.com/phillipjad/Aurify/issues/68)).
- **Admin tooling for lockouts** — a permanently blocked `(IP, address)` pair
  can currently only be cleared by an operator deleting the row
  ([ADR 0012](docs/adr/0012-account-lockout-policy.md)). Authentication itself
  is built: email/password and Sign in with Google, with rotating refresh
  tokens ([ADR 0011](docs/adr/0011-authentication-and-sessions.md)).
- **Lyrics + sentiment** — both real: lyrics come from lrclib, and sentiment reads
  VADER's lexicon as a polarity ratio, aggregated as a proportion over the tracks
  that actually had lyrics
  ([ADR 0026](docs/adr/0026-lyric-sentiment-from-a-weighted-lexicon.md)). Coverage
  is the remaining limit, not the method: a playlist lrclib does not know
  contributes no lyric signal rather than a neutral one.
- **Audio features** — measured, from AcousticBrainz where a track matches
  ([ADR 0018](docs/adr/0018-audio-features.md)). All seven palette dimensions
  now read a measured signal: `intimate` was dropped because nothing reachable
  measures speech, and the bright/dark pair reads major/minor rather than a mean
  of mood classifiers ([ADR 0025](docs/adr/0025-palette-from-measured-signals.md)).

## Deploying

A single container on Cloud Run, secrets in Secret Manager, PostgreSQL on Neon,
all defined in OpenTofu under [`deploy/`](deploy) and applied by the `Deploy`
GitHub Actions workflow. The bootstrap steps are in
[`deploy/README.md`](deploy/README.md).
