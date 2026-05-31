# 0007 — fgrzl/mux as the web framework

- Status: Accepted
- Date: 2026-05-30

## Context

The API needs routing, request binding, typed responses, middleware, health
probes, and ideally OpenAPI generation. The project selected
[`fgrzl/mux`](https://github.com/fgrzl/mux) for this.

## Decision

Use `fgrzl/mux` (v0.2.0) as the web framework. Patterns adopted:

- Build with `mux.NewRouter()` then `router.Configure(func(r *mux.Router){...})`
  so route registration is validated up front.
- Group the API under `r.Group("/api/v1")`; handlers have the signature
  `func(c mux.RouteContext)` and use `c.Bind`, `c.Params().String`,
  `c.Query().Int`, and typed responses (`c.OK`, `c.Created`, `c.NotFound`,
  `c.BadRequest`, `c.ServerError`).
- Path params use brace syntax: `/covers/{id}`.
- Use built-in middleware (`mux.UseLogging`, `mux.UseCompression`,
  `mux.UseCORS`) and probes (`r.Livez()`, `r.ReadyzWithCheck(...)`).
- Decorate routes with OpenAPI metadata (`WithOperationID`, `WithSummary`,
  `WithJSONBody`, `WithOKResponse`, ...). The framework can emit a spec via
  `mux.GenerateSpecWithGenerator`, which we can use to generate the frontend's
  API types.

## Consequences

- A batteries-included framework (auth, rate limiting, OpenTelemetry, OpenAPI)
  we can grow into; e.g. `mux.UseAuthentication` will replace the scaffold's
  `X-User-ID` header convention.
- **Risk:** `fgrzl/mux` is pre-1.0; its API may change between releases. We pin
  the version and isolate all usage in `internal/transport/http`, so a future
  migration (or swap to `net/http` + chi) is contained to one package.
- pkg.go.dev does not render its docs (license restriction); read the source /
  `examples/` directory when extending usage.
