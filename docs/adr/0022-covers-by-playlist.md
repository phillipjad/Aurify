# 0022 — A cover is a playlist, and each generation is a revision of it

- Status: Accepted
- Date: 2026-08-09

## Context

`covers` had one row per generation run and no `UNIQUE (user_id, platform,
playlist_id)`, so regenerating a playlist added a second tile rather than
updating the first. The dev library held 95 rows across 25 distinct playlists,
with one playlist alone accounting for 21. The gallery read as a log of runs
where it should read as a library of playlists.

The gallery papered over the symptom with a `dedupeById` helper and offset
pagination, neither of which can express "one tile per playlist".

## Decision

**Split the run out of the identity: `covers` is one row per playlist,
`cover_revisions` is one row per finished run.**

```
covers                       -- the identity
  id, user_id, platform, playlist_id   UNIQUE (user_id, platform, playlist_id)
  playlist_name
  status, error              -- the newest run's lifecycle, including in-flight
  created_at, updated_at

cover_revisions              -- one row per *finished* run
  id, cover_id               -- FK, ON DELETE CASCADE
  revision_number            -- UNIQUE (cover_id, revision_number)
  status                     -- terminal only: ready | failed
  prompt, analysis, error
  completed_at
```

`cover_images` re-keys from `cover_id` to `revision_id`, so each run keeps its
own bytes and they cascade with it.

### Revisions are written only at a terminal status

A revision row is created once, on `ready` or `failed`. In-flight lifecycle
lives solely on `covers`, which is what leaves the SSE path untouched:
`covers_notify_update` still fires on every pipeline write, so `GET
/covers/{id}/events`, the `FailStuck` sweep and the analyzing-phase heartbeat all
work unchanged (ADR 0019). Two consequences, both accepted:

- `completed_at` is the finish time, not the start time.
- A run interrupted by a crash or scale-down leaves **no** revision. The sweep
  marks the cover failed and history records nothing, because nothing was
  produced.

### The current artwork is a query, not a pointer

`prompt`, `image_url` and `analysis` left `covers` entirely. A cover's artwork is
read as "the newest revision with `status = 'ready'`", via a `LEFT JOIN LATERAL`.
During a regeneration that is automatically still the previous run's, so *old
artwork while regenerating* falls out with no promotion step, no
`current_revision_id` to maintain when a revision is deleted, and no circular
foreign key. Deleting the newest successful run falls back to the one before it
for the same reason; deleting the last leaves the tile on the placeholder.

The cost is a lateral join per row on the list query. Add the denormalised
pointer only if it shows up in a query plan.

### Image URLs name the run, not the cover

`imageUrl` is `/api/v1/covers/{revisionId}/image`. Keyed by the cover it could
never be cached, because a regeneration would change what it served; keyed by the
run, the bytes behind a URL never change, and the immutable `Cache-Control` the
route already sets becomes true rather than aspirational.

There is no `image_url` column on the revision. The storage adapter builds the
URL from the revision id on read, which is the same package that decides where
the bytes live, so the ADR 0016 seam holds. `ImageStore.Put` no longer returns a
URL nobody was going to keep.

### Keyset pagination

`GET /covers` pages on `(updated_at DESC, id DESC)` rather than an offset. A
regeneration reorders its playlist to the front mid-scroll, and an offset then
repeats or skips a tile. `id` is the tiebreaker because `updated_at` can collide.
`idx_covers_user_created` becomes `(user_id, updated_at DESC, id DESC)`.

The cursor is read off the last row of the previous page rather than returned in
an envelope: every cover already carries `updatedAt` and `id`, so the list
response keeps its shape. `dedupeById` is gone — with a unique constraint and
keyset paging, a duplicate id in one list is a bug worth seeing.

### Toasts are keyed by run

`cover.id` now names the playlist, so it cannot identify an announcement. The
fire-once guard in `cover-toasts.tsx` keys on `${id}:${updatedAt}` instead, and
passes the same value to Sonner. Guarding on the cover id would have made every
regeneration finish in silence, and told Sonner to restyle the previous card
rather than raise a new one. `updatedAt` works because every pipeline stage
stamps it: its terminal value names exactly one run, and a replayed snapshot of
that run repeats it exactly.

### One run in flight per cover

A cover may have exactly one generation running. The exclusion is the claiming
write itself — `StartCoverRun`, an `INSERT ... ON CONFLICT DO UPDATE` whose
update is conditional on `covers.status IN ('ready', 'failed')` — and not a check
in front of a write, which two callers would both pass.

That predicate is atomic because of how PostgreSQL handles a conflicting
`ON CONFLICT DO UPDATE`: it locks the existing row and re-evaluates the
condition against the latest committed version, so a second caller either waits
for the first and then sees a non-terminal status, or loses the unique-index race
and lands on the same predicate. No row comes back, and the repository turns that
into `domain.ErrGenerationInFlight` → `409`. It holds across API instances,
because the guarantee lives in the database rather than in a process.

Two overlapping runs were the alternative, and they are not merely wasteful: they
interleave status writes on one row, so the run that finishes second can leave
the cover non-terminal after the other has already written `ready`, stranding it
until the sweep.

`Save` keeps its unconditional upsert: it is the owning pipeline moving its own
row forward, including back to terminal.

### Releasing a claim quickly (amends ADR 0019)

A claim is released by reaching a terminal status, which every exit from the
pipeline does. What matters is how fast it is released when the pipeline never
gets there. Under ADR 0019's sweep — 10-minute staleness, 5-minute interval —
that was up to ~15 minutes with the playlist unusable throughout. Three changes
bring it to seconds in the common case, and each addresses a distinct way a run
can stop.

**The process died.** Startup reaps every non-terminal cover outright, with no
staleness threshold, because `deploy/app/main.tf` now pins
`max_instance_count = 1`: generation is in-process, so a claim can only be held
by a goroutine in *this* process, and nothing that was running before the restart
is running after it. Combined with scale-to-zero, a stuck cover is close to
unobservable — nobody can reach one without waking the instance that reaps it on
the way up. The reap is synchronous and runs before the server accepts anything,
so it cannot race a claim this process just made.

The cost is horizontal scale. It is not needed here: a generation is mostly
waiting on rate-limited upstreams, not CPU. Raising the instance cap means giving
the reap a threshold again, or a booting instance will fail its peers' live runs.
A deployment briefly overlaps two revisions and the new one will reap the old
one's run, which is the right answer anyway: that instance is draining and
shutdown does not wait.

**The run stopped without the process dying.** The sweep stays, at a 2-minute
threshold on a 30-second timer. That is affordable because the heartbeat now
covers the *whole* run rather than only `analyze`. The old 10-minute threshold
was not conservatism about the beat — the `generating` phase had no beat at all,
and it spans a 60s prompt call and a 120s image call, so the threshold had to
clear the slowest upstream. Now it answers to the beat: 30 seconds, four missed
beats before a live run is at risk.

The beat is `Touch` — `UPDATE covers SET updated_at = now() WHERE id = $1` — and
takes the cover id rather than the cover. That is what makes it safe to run
beside the pipeline for a whole generation: the id is fixed once claimed, while
the cover struct is mutated stage by stage, so passing the struct would be a data
race and a beat could write back a stage the run had already left. A partial
index on `(updated_at) WHERE status IN (...)` keeps the 30-second sweep reading
only rows that are actually in flight.

Beating on `updated_at` means each beat fires the notify trigger and every
watcher re-receives the snapshot it has. That costs a parse and a render the
client was already doing every second for the stage label. A separate
`heartbeat_at` column with a `WHEN` clause on the trigger silences it, and is
worth doing only if anyone notices.

**The run never returns.** Neither of the above helps here: the heartbeat is what
keeps such a run alive, so it holds its claim for as long as the process does.
`maxRunDuration` (15 minutes) caps the pipeline's context. The outcome is written
through a detached context, because the most likely reason to be recording a
failure is that the context is the thing that died — writing through the expired
one would fail both writes and leave the cover claimed, which is the wedge the cap
exists to remove.

### Failed runs are hidden by default

They are recorded, because their error and their stored prompt are the only
evidence explaining an image-provider refusal, but `GET /covers/{id}` returns
successful renders unless
`?allow_failed_revisions=true`, and the tile's run count follows the same
default. This is separate from the gallery's `All | Ready | Failed` filter, which
reads `covers.status` and therefore means "the newest run's outcome".

### Backfill

Migration `00008` runs in one transaction: every terminal row becomes a revision
keeping its own id, `cover_images` is renamed onto `revision_id`, the duplicate
covers are deleted keeping the newest per playlist, and only then is the unique
constraint added. Keeping the old ids is what makes this cheap — the image bytes
need no rewriting, existing `imageUrl` values still resolve, and `/covers/{id}`
links to each playlist's most recent generation keep working.

Run against a copy of the dev database: 95 rows collapsed to 25 covers and 95
revisions, all 78 image rows preserved, no duplicates remaining. The rename also
preserves the `STORAGE EXTERNAL` setting from migration 00005; recreating the
table would have dropped it silently.

### Revisions are numbered by the server, not by position

A revision's label ("Revision #28") comes from a stored `revision_number`,
assigned as `MAX + 1` within the cover. Counting position in the list was the
alternative and it is not stable: deleting one run renumbers every older one, so
a label on an open page and a `?rev=` link would both start pointing at different
artwork. Failed runs take a number too, since skipping them would reintroduce the
same instability by another route, and a deleted run leaves a gap rather than
freeing its number for reuse.

`MAX + 1` is safe without a sequence because only one run per cover is ever in
flight, which `StartCoverRun` enforces; `UNIQUE (cover_id, revision_number)` is
what makes that a guarantee instead of an assumption. The number is deliberately
absent from the upsert's `DO UPDATE` list: the failure path re-writes a revision
already recorded as ready, and that is one run correcting its own outcome.

### The prompt is internal

The generation prompt is stored and never returned by the API, on a cover or on a
revision. It is an implementation detail of how artwork gets made; keeping it in
Postgres is what allows diagnosing a provider refusal after the fact, and the
client has no use for it that justifies exposing the wording Aurify sends
upstream. The run history shows renders, dates and errors instead, which is what
distinguishes one run from another anyway.

### Browsing revisions

Two mechanisms, deliberately: a chevron stepper under the artwork for moving one
run at a time, and a scrollable thumbnail list, in the space the prompt vacated,
for jumping. The stepper walks successful runs only, because a failed one has no
artwork and stepping onto it would empty the frame the stepper sits under; failed
runs stay in the list behind the existing toggle, where their error is the useful
thing to show.

The selected revision lives in `?rev=`, so a refresh, the back button and a
shared link all survive. A number naming a run that never existed or has since
been deleted falls back to the current artwork rather than erroring.

Viewing is not restoring. "Current" is derived (the newest ready revision), so a
restore action would mean adding the `current_revision_id` pointer this design
deliberately avoids, and every reader that resolves artwork would have to follow
it. Download an older run instead.

### No retention

95 runs is roughly 12MB of bytes in PostgreSQL at ~150KB per JPEG. A per-cover
cap is the obvious lever if a regeneration habit makes this real, at the cost of
silently truncating the history this change exists to keep.

## Consequences

- The grid shows each playlist once, carrying its latest artwork, and
  regenerating updates that tile in place without blanking it.
- Every prior successful run stays reachable from the detail page, with its own
  artwork and palette, through the stepper or the list. Two delete actions now
  exist: one run, or the playlist and every run with it.
- `prompt` is gone from every API response, and `number` is a new required field
  on a revision. Both are breaking changes to a contract nothing outside this
  repo consumes yet.
- `POST /covers` can now answer `409`: the playlist is already generating. The
  detail page's Regenerate control is disabled while a run is in flight, so that
  is a race-loser's answer rather than something a user meets by clicking.
- A generation killed mid-run holds its playlist for seconds (restart) or up to
  ~2.5 minutes (sweep) rather than up to ~15, and a run that hangs is capped at
  15 minutes instead of never ending. ADR 0019's sweep timings are amended.
- The service no longer scales horizontally. That is a deliberate trade for the
  reap above, and the thing to revisit first if load ever justifies it.
- A heartbeat every 30 seconds per in-flight cover: one `UPDATE` and one SSE
  snapshot per watcher.
- The run count and `updatedAt` are new required fields on `CoverResponse`.
- A cover with no successful run yet has no artwork at all, which is the same
  placeholder the gallery already renders for a pending one.
