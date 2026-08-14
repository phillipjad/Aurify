# Architecture Decision Records

This directory records the significant architecture decisions for Aurify, using
lightweight [MADR](https://adr.github.io/madr/)-style records.

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0002](0002-monorepo-layout.md) | Monorepo layout: `backend/` + `frontend/` | Accepted |
| [0003](0003-lightweight-cqrs.md) | Lightweight CQRS in the application layer | Accepted |
| [0004](0004-mongodb-storage.md) | MongoDB via mongo-driver/v2 for persistence | Superseded by [0010](0010-postgresql-storage.md) |
| [0005](0005-dsp-provider-abstraction.md) | DSP provider abstraction + normalized model | Accepted |
| [0006](0006-llm-sidecars.md) | Local LLM sidecars for prompt + image generation | Superseded by [0014](0014-hosted-generation-apis.md) |
| [0007](0007-fgrzl-mux-web-framework.md) | fgrzl/mux as the web framework | Accepted |
| [0008](0008-openapi-contract.md) | OpenAPI as the API contract; generated frontend types | Accepted |
| [0009](0009-youtube-music-via-data-api-v3.md) | YouTube Music via the YouTube Data API v3 | Accepted |
| [0010](0010-postgresql-storage.md) | PostgreSQL (relational + JSONB) via pgx + sqlc | Accepted |
| [0011](0011-authentication-and-sessions.md) | Hand-built auth: Ed25519 access tokens + opaque refresh tokens | Accepted |
| [0012](0012-account-lockout-policy.md) | Warn-then-block lockout on the (IP, address) pair | Accepted |
| [0013](0013-secrets-and-deployment.md) | Secrets in Secret Manager, injected as env vars on Cloud Run | Accepted |
| [0014](0014-hosted-generation-apis.md) | Hosted APIs for prompt + image generation, called from the API | Accepted |
| [0015](0015-lyrics-cache-and-failure-policy.md) | Lyrics cache, bounded concurrency, and a skip-don't-retry failure policy | Accepted |
| [0016](0016-generation-providers.md) | Generation providers: an OpenAI-compatible base URL, and images in PostgreSQL | Accepted |
| [0017](0017-bounded-app-shell.md) | The app shell is bounded to the viewport, with `<main>` as the only scroll container | Accepted |
| [0018](0018-audio-features.md) | Audio features from AcousticBrainz, estimated when absent | Amended by [0020](0020-coverage-weighted-features.md), [0023](0023-pace-from-onset-rate.md), [0024](0024-feature-cache-versioning.md), [0025](0025-palette-from-measured-signals.md) |
| [0019](0019-async-cover-generation.md) | Cover generation off the request, progress over SSE | Amended by [0022](0022-covers-by-playlist.md) |
| [0020](0020-coverage-weighted-features.md) | Weight the measured palette by coverage, estimate from lyrics | Accepted |
| [0021](0021-patched-sonner-unmount-delay.md) | Sonner is patched so toast exits can finish | Accepted |
| [0022](0022-covers-by-playlist.md) | A cover is a playlist, and each generation is a revision of it | Accepted |
| [0023](0023-pace-from-onset-rate.md) | Pace from onset rate, not from BPM | Accepted |
| [0024](0024-feature-cache-versioning.md) | Invalidate the feature cache by version, not by delete | Accepted |
| [0025](0025-palette-from-measured-signals.md) | A palette of signals the sources actually measure | Amended by [0026](0026-lyric-sentiment-from-a-weighted-lexicon.md) |
| [0026](0026-lyric-sentiment-from-a-weighted-lexicon.md) | Lyric sentiment from a weighted lexicon, aggregated as a proportion | Accepted |

## Adding a new ADR

Copy an existing file, bump the number, and set the status to `Proposed`. Once
agreed, change it to `Accepted` and add a row above. Superseded ADRs keep their
file and link forward to the replacement.
