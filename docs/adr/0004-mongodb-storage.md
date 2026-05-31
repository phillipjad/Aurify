# 0004 — MongoDB via mongo-driver/v2 for persistence

- Status: Accepted
- Date: 2026-05-30

## Context

We need to persist users (with their DSP connections) and the covers they
generate. Covers embed a rich, nested `PlaylistAnalysis` (mean features, mean
sentiment, weighted palette) whose shape will evolve as the analysis improves.

## Decision

Use **MongoDB 8.0** with the official **`mongo-driver/v2`**. Two collections:

- `users` — `domain.User`, keyed by `_id` (a hex string we generate).
- `covers` — `domain.Cover`, with the analysis embedded as a sub-document.

Repositories live in `internal/storage/mongo` and implement
`ports.UserRepository` / `ports.CoverRepository`. Domain structs carry `bson`
tags directly (a deliberate simplification for the scaffold).

## Consequences

- The flexible document model fits the nested, evolving analysis payload without
  migrations or join tables.
- `bson` tags on domain types couple the domain to the storage encoding. Tolerable
  now; if it becomes a problem, introduce persistence-specific structs in the
  mongo package and map to/from domain types.
- v2 API differences to remember: `mongo.Connect` takes no context; the old
  `primitive` package is merged into `bson` (`bson.ObjectID`, `bson.NewObjectID`);
  options use builders (e.g. `options.Replace().SetUpsert(true)`).
- Indexes (e.g. `covers.user_id + created_at`, unique `users.email`) are not yet
  created in code — add an index-ensuring step on startup.
