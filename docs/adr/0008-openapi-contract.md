# 0008 — OpenAPI as the API contract; generated frontend types

- Status: Accepted
- Date: 2026-05-31

## Context

The backend defines the wire format in Go DTOs
(`backend/internal/transport/http/dto`); the frontend needs matching TypeScript
types. Hand-maintaining both sides invites drift: a renamed or added field on
one side silently diverges from the other. `fgrzl/mux` can emit an OpenAPI spec
from its route metadata, which we already attach (`WithJSONBody`,
`WithOKResponse`, path/query params, etc.).

## Decision

Treat the **OpenAPI spec as the single source of truth** for the HTTP contract,
generated from the live routes — never hand-written.

- **Backend:** `backend/cmd/openapi` builds the real mux router (with no live
  dependencies) and writes the spec to `backend/api/openapi.yaml`. It is wired
  to `go generate`:

  ```bash
  cd backend && go generate ./...        # regenerates api/openapi.yaml
  ```

- **Frontend:** [`openapi-typescript`](https://github.com/openapi-ts/openapi-typescript)
  turns that spec into `frontend/src/lib/api/schema.ts`:

  ```bash
  cd frontend && pnpm gen:api            # reads ../backend/api/openapi.yaml
  ```

  `src/lib/api/types.ts` then derives its exported types from
  `components['schemas']` — it no longer re-declares field shapes.

Both generated artifacts (`api/openapi.yaml`, `schema.ts`) are committed so a
fresh clone type-checks without running codegen first (same convention as
TanStack Router's `routeTree.gen.ts`).

DTO struct tags carry the contract richness (mux reads them via
`fgrzl/json/jsonschema`):

- `binding:"required"` on always-present fields → the schema's `required` array
  → non-optional TypeScript properties. `omitempty` fields stay optional.
- `enum:"a,b,c"` on the `platform`/`status` fields → the schema's `enum` → the
  client generates a TS string union. Those fields are typed in Go as
  `domain.DSPPlatform` / `domain.CoverStatus`, and `types.ts` derives `Platform`
  and `CoverStatus` from the generated schema — no hand-written unions.

The end-to-end workflow when the contract changes: edit the Go DTO/route (and
its tags) → `go generate ./...` → `pnpm gen:api`.

## Consequences

- The contract flows one way (Go DTOs + tags → spec → TS); renaming/adding a
  field or enum value is a two-command propagation, and a stale client surfaces
  as a type error.
- The route registration in `internal/transport/http` doubles as the API
  documentation; keep the `With*` OpenAPI decorations accurate.
- Enum values live in two places — the domain constants and the `enum:"..."`
  tags (tags must be string literals, so they can't reference the constants).
  Keep them in sync; the DTO comment flags this.
- **CI recommendation:** run `go generate ./... && pnpm gen:api` and fail if the
  working tree changes, to guarantee the committed artifacts match the code.
