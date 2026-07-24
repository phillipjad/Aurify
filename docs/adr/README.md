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
| [0006](0006-llm-sidecars.md) | Local LLM sidecars for prompt + image generation | Proposed |
| [0007](0007-fgrzl-mux-web-framework.md) | fgrzl/mux as the web framework | Accepted |
| [0008](0008-openapi-contract.md) | OpenAPI as the API contract; generated frontend types | Accepted |
| [0009](0009-youtube-music-via-data-api-v3.md) | YouTube Music via the YouTube Data API v3 | Accepted |
| [0010](0010-postgresql-storage.md) | PostgreSQL (relational + JSONB) via pgx + sqlc | Accepted |
| [0011](0011-authentication-and-sessions.md) | Hand-built auth: Ed25519 access tokens + opaque refresh tokens | Accepted |
| [0012](0012-account-lockout-policy.md) | Warn-then-block lockout on the (IP, address) pair | Accepted |
| [0013](0013-secrets-and-deployment.md) | Secrets in Secret Manager, injected as env vars on Cloud Run | Accepted |

## Adding a new ADR

Copy an existing file, bump the number, and set the status to `Proposed`. Once
agreed, change it to `Accepted` and add a row above. Superseded ADRs keep their
file and link forward to the replacement.
