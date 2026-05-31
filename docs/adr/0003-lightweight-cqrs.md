# 0003 — Lightweight CQRS in the application layer

- Status: Accepted
- Date: 2026-05-30

## Context

The product has a clear asymmetry between writes and reads. Writes are few but
involved (link a DSP account; run the analyze→prompt→image pipeline to produce a
cover). Reads are simple and frequent (list playlists, list/get covers). We want
that asymmetry to be visible in the code without the ceremony of full event
sourcing.

## Decision

Adopt **lightweight CQRS** over a single MongoDB:

- `internal/app/command/` — the write side. One sub-package per use case
  (`connectdsp`, `generatecover`), each a single-purpose `Handler`. A `command.Bus`
  struct aggregates them.
- `internal/app/query/` — the read side. One sub-package per read use case
  (`listplaylists`, `getcover`, `listcovers`), same shape, collected in a
  `query.Bus`.
- `app.App{ Commands, Queries }` is the single dependency the transport layer
  takes.

There is **no** event store, no separate read database, and no eventual
consistency. Handlers depend on interfaces (`internal/app/ports`) and are wired
to concrete adapters in `cmd/api`.

## Consequences

- The read/write split is obvious and each use case is independently testable.
- Adding a use case = adding a sub-package + a field on the relevant Bus.
- We forgo event-sourcing benefits (audit log, rebuildable projections). If
  those become requirements, this structure migrates cleanly: commands already
  isolate writes, so an event store can be introduced behind the existing ports.
- Slight boilerplate from one package per use case; accepted for clarity.
