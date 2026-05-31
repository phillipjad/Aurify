# 0002 — Monorepo layout: `backend/` + `frontend/`

- Status: Accepted
- Date: 2026-05-30

## Context

Aurify has two deployable artifacts written in different languages: a Go API and
a React PWA. They share a domain vocabulary (playlists, tracks, covers,
analysis) but no runtime code.

## Decision

Use a single repository with two top-level, self-contained directories:

- `backend/` — a Go module (`github.com/phillipjad/aurify/backend`) with its own
  tooling (`go.mod`, `Makefile`, `.golangci.yml`).
- `frontend/` — a pnpm/Vite+ project (`@aurify/web`) with its own `package.json`.

Cross-cutting concerns (README, AGENTS.md, ADRs, `docker-compose.yml`) live at
the repo root.

## Consequences

- Simple mental model; each side builds and is tooled independently.
- No shared-package build graph to maintain (the alternative, a pnpm/Nx
  workspace with `apps/*` + `packages/*`, was rejected as premature).
- The API ↔ client contract is duplicated (TS types mirror Go DTOs). We accept
  the small drift risk for now and can later generate the TS types from the
  API's OpenAPI spec (see ADR 0007). If shared TS packages become necessary,
  revisit and migrate to a workspace.
