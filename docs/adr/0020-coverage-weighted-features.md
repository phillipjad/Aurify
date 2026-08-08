# 0020 — Weight the measured palette by coverage, estimate from lyrics

- Status: Accepted
- Date: 2026-08-07
- Amends: [0018](0018-audio-features.md) (its fallback rule and its lookup cap)

## Context

[ADR 0018](0018-audio-features.md) fell back to the title-based estimate only
when *nothing* matched (`AnalyzedCount == 0`). One matched track was therefore
treated as authoritative. Measured on the dev library, 33 stored analyses:

| Playlist | matched / tracks | coverage | recorded danceability |
|---|---|---|---|
| EDM | 6 / 192 | 3% | 1.00 |
| 3arc | 2 / 34 | 6% | 0.53 |
| Chill Stratovarius | 2 / 7 | 29% | 0.99 |
| "I don't know what they're saying…" | 1 / 2 | 50% | 1.00 |
| CN:PTAMF | 1 / 2 | 50% | acousticness 0.97 |

A mean over two samples of a bounded [0,1] variable has a standard error near
0.2. `danceability 0.99` for power metal is that error, not a measurement, and
the old rule preferred it over an estimate purely for being non-zero.

The estimator had never run: `features_estimated` is null on all 33 rows.

## Decision

**Blend the measurement toward the estimate in proportion to coverage, estimate
from lyrics rather than titles, and lift the lookup cap now that the phase it
governs is pipelined.**

### The weight

`analysis.CoverageWeight` is `min(1, share/0.5, matched/8)`: a measured mean
stands alone once it covers half the playlist **and** rests on eight tracks.

Both terms are load-bearing and each catches what the other cannot. Share alone
passes 1-of-2 at full weight, which is the original bug restated. Count alone
blends a well-covered short playlist against a guess for being short. The
consequence accepted with the count term is that a fully matched three-track
playlist is still blended at 3/8: it clears the share floor, but AcousticBrainz's
own per-track classifier error does not average out over three tracks.

`AnalyzedCount` and `TrackCount` are already persisted, so the weight of any
stored analysis is reconstructible; no new field records it. `FeaturesEstimated`
widens from "the guess is the whole answer" to "a guess contributed".

`TempoBPM` is never blended. The estimator answers in [0,1] and has no opinion
on tempo, so mixing its zero in would drag a real BPM down.

### Lyrics, not titles

The pipeline resolves lyrics one step before the estimator runs, so they cost
nothing to reach and say more about a track than its title. Measured against
gemma3:4b, mean absolute error over the four dimensions AcousticBrainz actually
supplies, three runs per prompt:

| Set | tracks | measured | titles only | with lyrics |
|---|---|---|---|---|
| Stratovarius (power metal) | 8 | 4 | 0.447 | **0.354** |
| Kanye / Kendrick / Drake | 21 | 12 | **0.311** | 0.348 |

Lyrics help the obscure set and hurt the well-known one, which is coherent: for
an artist the model already knows, the title alone is the stronger signal and
lyrics dilute it. The estimator only runs below the coverage floors, and thin
coverage correlates with obscure or post-2022 catalogues — the regime where
lyrics measured better. This is two usable sets and it is not a large margin.

A third set (Illenium / k?d / DROELOE / Crankdat) is omitted rather than
reported: its "measured" baseline was a single track, which is the very noise
this ADR exists to correct.

Treat the absolute errors as soft for the same reason. Both baselines are
partial samples of their sets, so they carry the sampling bias described below
under Stratovarius. What they support is the *comparison* — both prompts were
scored against the same baseline — not a claim about how close either is to the
truth.

Prompt budget: nothing in `internal/platform/llm` sets `num_ctx`, so the ceiling
is the provider's default (4k on Ollama), not the model's capability.
`maxTracksInPrompt` drops 40 → 20 and each lyric is excerpted to ~600 characters
on whole lines. A track lrclib found nothing for contributes its title line
alone, so a playlist without lyrics degrades to the previous behaviour.

### The cap, and pipelining

`maxLookupsPerRun` rises 25 → 100. The cap stopped protecting request latency
when generation moved off the request ([ADR 0019](0019-async-cover-generation.md));
what it governs now is how long a user watches the analyzing stage.

Measured cold against the real APIs, uncached tracks from the dev library:

| | 25 lookups | 100 lookups |
|---|---|---|
| Sequential | 1m6s (2.6s/track) | 3m35s (2.15s/track) |
| Pipelined | — | **1m45s (1.05s/track)** |

The handoff's "analyzing ≈ one second per lookup" does not reproduce. Only the
MusicBrainz search is throttled, at 1 request/second/IP; the rest of each 2.15s
was two sequential HTTP round trips nothing was limiting. `Client.resolveAll`
now runs the searches on their own goroutine against the rate limiter while the
consumer spends that second on the previous track's AcousticBrainz fetch, which
brings the phase to the rate limit's own floor and makes a cap of 100 cost less
than the old cap of 25 did.

**MusicBrainz has no batch search**, checked against the API docs and against
the service. A Lucene `OR` of several tracks is expressible and returns one flat
relevance-ordered list — 161 results for two tracks — with no way to attribute a
row back to the track that asked for it, and `limit` caps at 100 results for the
whole query. A batch would have to re-match client-side on the artist and title
it already knew, which is the fuzzy step the per-track search avoids.

### Keeping the row alive

The analyzing phase writes nothing of its own: `run` saved the cover as
`analyzing` and did not save again until `generating`, leaving `updated_at`
frozen across lyric resolution and every lookup. The sweep in `cmd/api` fails
non-terminal covers idle for ten minutes, so the ceiling on that phase was a
deadline rather than a choice. Steps 1–4 move into `Handler.analyze`, which
starts a one-minute heartbeat and defers its stop, so every exit path stops the
writer and the writer is provably dead before `run` touches the cover again.

### A word that moves

Both waiting stages cycle through synonyms — eight for listening, seven for
painting — each holding for one full ellipsis before handing over: "Listening",
"Listening.", "Listening..", "Listening...", "Judging". A period a second, a verb
every four. Frontend only: no progress count is persisted, streamed, or added to
the contract.

The cycle is anchored to the moment a stage begins rather than to the wall clock.
Anchored to the clock, a generation opened on whatever word the time of day
landed on, and crossing pending → analyzing → generating re-derived the index
against a differently-sized verb list each time, so a fresh click appeared to
race through several words before settling. Two views still agree in practice,
because both reset on the same status event; only a view that mounts mid-stage
starts its own count, which is the price of never opening mid-word.

Nothing moves but the dots. They grow into a box already wide enough for three,
and the label reserves the widest verb (measured: "Composing" at 5.11em), so the
word's left edge is fixed for the whole stage rather than re-centring on every
tick. Same reserve in `em` for the badge at 12px and the button at 14px.

The placeholder mark on a generating tile carries an aurora that drifts through
it over 6s, rather than the whole glyph fading in and out. Colour moving through
the mark says "being made"; a dimmer says only "waiting".

That needs a fill an SVG stroked with `currentColor` cannot have, so `CoverGlyph`
renders the mark as a CSS mask over a painted box and lets the gradient animate
underneath. The gradient is the resting muted colour at both ends with the colour
band in the middle, so the sweep starts and finishes on the plain icon and any
stop rests there too.

It carries `data-motion="aurora"`, joining the spinner in ADR 0019's
busy-indicator exemption from the reduced-motion reset, on the same reasoning: it
is the only thing on an artwork-less tile saying the tile is still being worked
on, and a still one is indistinguishable from a cover that never arrived. It
keeps 6s rather than borrowing the spinner's 1.8s, since a drift with no travel,
flash or bounce is already the gentlest of these.

This is a sign of life, not progress. A user at 1m45s sees motion but not how far
along they are, which is the accepted cost of not adding a progress field to the
row, the DTO, the OpenAPI contract and the client for a number meaningful during
one stage only.

The cycling word is `aria-hidden`, so fifteen synonyms a minute are never spoken.
What the `aria-live` region carries instead is the stable stage plus how long it
has been running, re-announced every thirty seconds: "Listening", then
"Listening, 30 seconds", then "Listening, 1 minute 30 seconds". Elapsed time
rather than progress because elapsed time is the one honest number this screen
has. Without it the badge announced once and then went silent for two minutes,
which is what a dead page sounds like — sighted users got motion and screen
reader users got nothing.

## Measured end to end

Three real generations against the live YouTube Music connection, real lyrics,
real AcousticBrainz and local gemma3:4b, with image generation stubbed:

| Playlist | matched / tracks | weight | before → after |
|---|---|---|---|
| Chill Stratovarius | 3 / 7 | 0.375 | danceability 0.66 → **0.34** |
| EDM | 36 / 192 | 0.375 | danceability 1.00 → **0.49** |
| ?!?Country?!? | 8 / 12 | 1 | unchanged, estimator not called |

EDM went from 6 matched tracks to 36 on the same playlist, purely from the cap,
and its whole pipeline ran in 1m12s. The heartbeat was observed firing once at
exactly 60 seconds into that 72-second analyzing phase.

**Acousticness on that playlist is the clearest case for the whole change.** The
3 matched tracks averaged `acousticness 0.03`, and the estimator said 0.85, so
the blend moved it to 0.54. It is tempting to read that as the estimator's known
"power metal is acoustic" error from ADR 0018 reaching the palette. The per-track
data says otherwise:

| track | measured acousticness |
|---|---|
| old man and the sea | 0.969 |
| forever | 0.096 |
| 4000 rainy nights / before the winter | 0.003 / 0.006 |
| coming home / dream with me / unbreakable | 0.000 |

The playlist is a *chill* selection, and it contains at least one genuinely
acoustic track — which the matched sample excluded, since including it could not
have produced a mean of 0.03. The sample was not a small view of the truth, it
was the wrong view of it, which is exactly the failure this ADR exists to
correct. 0.85 is not demonstrably right either; the point is that 0.03 was never
the reference it looked like.

Read the same way, the danceability column is a warning about the measurements
themselves: AcousticBrainz scores "coming home" and "dream with me" at 1.000
danceable and the ballad "forever" at 0.000. Per-track classifier error of that
size is a second reason a three-track mean cannot carry a palette, independent of
sampling.

`instrumentalness` and `speechiness` are also non-zero for the first time on a
blended playlist, since nothing measures them and the estimate is their only
source.

## Consequences

- 21 of the 33 stored analyses fall below full weight, so the estimator goes
  from never running to running on most generations. It is the first real
  exercise of `llm/features` outside unit tests.
- The estimator is disabled wherever `AURIFY_PROMPTGEN_URL` is empty:
  `New("")` returns `Present: false` without erroring, which is why
  `features_estimated` is null on every analysis stored before this change. It
  is now set in `backend/.env.dev`.
- A cold 100-track lookup is 1m45s, faster than the old 25-track cap was
  sequentially, so coverage roughly quadruples at no cost in wall time.
- Only four of the palette's six dimensions are ever measured: `track_features`
  stores danceability, energy, valence and acousticness. `instrumentalness` and
  `speechiness` are structurally zero on the measured path, so the estimate is
  the only thing that ever moves the introspective and intimate dimensions.
- `ports.FeatureEstimator.Estimate` takes lyrics, so the port is no longer
  satisfiable from tracks alone.
