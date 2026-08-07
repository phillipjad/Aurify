# 0019 — Cover generation off the request, progress over SSE

- Status: Accepted
- Date: 2026-08-06

## Context

`POST /api/v1/covers` ran the whole pipeline inside the request. Measured on a
real library: 92s cold for a 192-track playlist, 5s warm for a small one, the
cold time dominated by MusicBrainz at one request per second. The request was
at the mercy of the slowest upstream, the client's only progress signal was a
3-second list poll, and the 25-lookup AcousticBrainz cap (`maxLookupsPerRun`)
existed only to keep the synchronous request bearable — it is the coverage
ceiling #73 wants lifted.

## Decision

**Accept-then-background, with progress pushed over Server-Sent Events. No new
job infrastructure, no new dependencies, and the client polls nothing.**

### The job

- `Handle` in `generatecover` validates the DSP connection, persists the cover
  as `pending`, and returns its id. The pipeline runs in a goroutine under
  `context.WithoutCancel(ctx)`, recording progress and failure on the cover
  row exactly as before.
- The cover row is the job record. `domain.Cover` already carries the full
  `pending → analyzing → generating → ready | failed` lifecycle. The outbox
  table the issue floated would be a second source of truth for state the row
  already holds, so it was not built.

### The push path

SSE over WebSockets, as the issue proposed: the traffic is one-directional,
survives ordinary HTTP infrastructure, and `EventSource` reconnects on its
own. The whole path is dependency-free — browser-native `EventSource`, stdlib
`net/http` streaming, and PostgreSQL `LISTEN/NOTIFY` through the pgx driver
already in use.

- A database trigger (`covers_notify_update`, migration 00007) fires
  `pg_notify('cover_updates', id)` on every cover write. A trigger rather than
  application code so every writer fires it: the pipeline, the stuck-cover
  sweep, and anything added later.
- Each API instance runs one `CoverWatcher` holding one `LISTEN` connection
  and fanning signals out to in-process subscribers. The in-memory subscriber
  map is safe across instances because it is only fanout: the state lives in
  the row, and a second instance runs its own listener against the same
  channel.
- `GET /covers/{id}/events` streams complete `CoverResponse` snapshots — one
  on connect, one per change — until the status is terminal, then closes.
  Full snapshots are what make reconnects trivial: the first event after a
  reconnect catches the client up entirely, so there is no `Last-Event-ID`
  replay log to maintain. Notifications carry only the id and the handler
  re-reads the row, so an event can never carry stale state.
- The frontend (`cover-events.ts`) opens one refcounted `EventSource` per
  in-flight cover, shared by the gallery, the playlist row, and the detail
  view; events are written into the TanStack Query cache and everything
  re-renders from there. Both former polls — the gallery's 3s
  `refetchInterval` and the per-row status poll — are gone.

### Auth on the stream

`EventSource` cannot set headers, so the stream authenticates by the same
access cookie as every other route and the normal middleware chain runs at
connect. That is one check on a long-lived connection, so the handler re-runs
the session revocation check on every 15s heartbeat: a session revoked
mid-stream terminates it rather than streaming on.

### fgrzl/mux accommodations

Neither its compression nor its logging wrapper can flush, so SSE has to reach
the raw `http.ResponseWriter`. `StreamPassthroughMiddleware` arranges that; the
mechanics are documented at the workaround itself.

### Cloud Run

By default Cloud Run allocates CPU only while a request is in flight, which
would throttle the background pipeline. `deploy/app/main.tf` sets
`cpu_idle = false`: CPU stays allocated for the instance's lifetime and the
service still scales to zero. Billing moves from per-request to
per-instance-lifetime, accepted as the smallest departure from ADR 0013's
scale-to-zero posture. The open SSE connection also counts as an in-flight
request, which keeps the instance alive while anyone is watching a generation.

### Interrupted work

A crash or scale-down orphans an in-flight cover in a non-terminal status. A
sweep in `cmd/api` runs at startup and every 5 minutes, failing non-terminal
covers untouched for 10 minutes (`FailStuck`); every pipeline stage bumps
`updated_at`, so a live job is never swept. The sweep's own write fires the
trigger, so a watching client is told about the failure the same way as any
other change. A restart fails interrupted covers rather than resuming them:
the pipeline is cheap to rerun.

## Consequences

- `POST /covers` returns a pending cover in well under a second regardless of
  playlist size, and status changes reach the client in the time it takes a
  NOTIFY to cross the wire, not at a 3-second poll boundary.
- No polling remains anywhere in the covers flow, and the API stops fielding
  one list request per client per 3 seconds during generation. Measured on a
  live session: 757 seconds with two list requests, both on mount.
- Because the stream reaches every route, a finished cover can announce itself
  anywhere. A toast fires on the ready event outside the covers section, with a
  link to the new cover; the streams are held from the root layout so they
  survive route changes, which is what makes that notification possible at all.
- Pipeline errors no longer reach the POST response; the cover row's `error`
  field travels over the stream and the playlist browser surfaces it.
- The `maxLookupsPerRun` cap is no longer load-bearing for request latency,
  unblocking #73.
- Each stream holds one server connection and the watcher holds one pool
  connection per instance. On HTTP/1.1 a browser caps same-host connections
  at ~6; sharing one stream per cover keeps normal use inside that, and HTTP/2
  in production multiplexes past it.
- A generation interrupted mid-run reports `failed` within at most ~15
  minutes (staleness threshold plus sweep interval).
