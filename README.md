# Aurify

> Cover art that captures the vibe of your playlist.

Users across music platforms have cherished playlists but rarely a cover that
captures their feel. **Aurify** fills that gap: log into your DSP (Spotify, Apple
Music, YouTube Music), pick a playlist, and Aurify analyzes every track — its
audio features *and* the sentiment of its lyrics — then generates an abstract
cover from a color palette weighted to how the music feels.

> **Status: scaffold.** This repository is a structured skeleton. The shapes,
> interfaces, and wiring are real and the backend builds; most external
> integrations (DSP network calls, the two LLM sidecars) are intentionally
> stubbed. See the per-component `README`s and `docs/adr/` for what is real vs.
> pending.

## How it works

```
DSP login ─▶ ingest tracks ─▶ per track: ─▶ aggregate + weight ─▶ prompt LLM ─▶ image LLM ─▶ cover
 (OAuth)     (normalized      audio features    into a color        (sidecar)     (sidecar)    (stored
             model)           + lyric sentiment palette                                         in Mongo)
                              via lrclib.net    [0,1] weights
```

Example of the weighting: a playlist with a mean liveness of `0.78` contributes
`0.78` (pre-normalization) to the *energetic* color; low valence plus negative
lyric polarity pushes weight toward the *melancholic* color. See
[`backend/internal/analysis/weights.go`](backend/internal/analysis/weights.go).

## Stack

| Layer | Choice |
|-------|--------|
| Backend | Go 1.26, [fgrzl/mux](https://github.com/fgrzl/mux), [mongo-driver/v2](https://github.com/mongodb/mongo-go-driver) |
| Storage | MongoDB 8.0 |
| Frontend | TypeScript 6, React 19, [Vite+](https://viteplus.dev/), [TanStack Query](https://tanstack.com/query) + [Router](https://tanstack.com/router), [Tailwind v4](https://tailwindcss.com), [shadcn/ui](https://ui.shadcn.com) |
| Architecture | Lightweight CQRS across backend & frontend (see [ADR 0003](docs/adr/0003-lightweight-cqrs.md)) |

## Repository layout

```
aurify/
├── backend/            # Go API (CQRS app layer, hexagonal ports/adapters)
├── frontend/           # React 19 PWA (Vite+, TanStack, Tailwind)
├── docs/adr/           # Architecture Decision Records
├── docker-compose.yml  # MongoDB (+ placeholders for the LLM sidecars)
├── AGENTS.md           # contributor & AI-agent guide
└── CLAUDE.md           # pointer for Claude Code
```

See [`backend/README.md`](backend/README.md) and
[`frontend/README.md`](frontend/README.md) for component details.

## Quick start

**Option A — full stack in Docker** (one command):

```bash
./start_container.sh -mb       # -m creates the Mongo data dir, -b builds images
                               # add -d to run detached; see ./start_container.sh --help
```

**Option B — native dev loop** (hot reload on the frontend):

```bash
# 1. Storage
docker compose up -d mongo

# 2. API  (http://localhost:8080)
cd backend && go mod tidy && go run build.go && ./bin/aurify

# 3. PWA  (http://localhost:5173, proxies /api -> :8080)
cd frontend && pnpm install && pnpm dev
```

Before committing, install the git hooks once: `lefthook install` (see
[AGENTS.md](AGENTS.md#git-hooks-lefthook)).

Toolchain versions: Go 1.26.3 · Node 24.16.0 · pnpm 11.5.0 · MongoDB 8.0.

## What's stubbed (and where to look)

- **DSP integrations** — interfaces + registry are real; network calls return
  "not implemented" ([`backend/internal/platform/dsp`](backend/internal/platform/dsp), [ADR 0005](docs/adr/0005-dsp-provider-abstraction.md)).
- **LLM sidecars** — HTTP clients with deterministic local fallbacks; the
  services aren't built ([`backend/internal/platform/llm`](backend/internal/platform/llm), [ADR 0006](docs/adr/0006-llm-sidecars.md)).
- **Auth** — handlers read an `X-User-ID` header until real sessions/JWT land.
- **Lyrics + sentiment** — the lrclib client is real; sentiment uses a naive
  lexicon placeholder.
