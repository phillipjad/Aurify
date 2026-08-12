# 0023 — Pace from onset rate, not from BPM

- Status: Accepted
- Date: 2026-08-11
- Amends: [0018](0018-audio-features.md) (adds a dimension and a second
  AcousticBrainz endpoint)

## Context

`domain.AudioFeatures.TempoBPM` existed, was written by nothing, and was read by
nothing. [Issue #77](https://github.com/phillipjad/Aurify/issues/77) proposed
filling it from AcousticBrainz's low-level `rhythm.bpm` and giving it a role in
the palette, on the reasoning that tempo is one of the strongest cues a listener
has for what a playlist feels like.

The reasoning holds. BPM is not the number that carries it.

Measured against the live API, 19 recordings split into two sets by their
published tempo:

| field | slow set (10) | fast set (9) | separation |
|---|---|---|---|
| published tempo | 72.8 | 158.2 | **+85.4** |
| `rhythm.bpm` | 123.1 | 121.9 | **-1.3** |
| `rhythm.bpm_histogram_first_peak_bpm` | 123.2 | 122.2 | -1.0 |
| `rhythm.onset_rate` | 2.37 | 3.46 | **+1.09** |
| `rhythm.danceability` | 1.00 | 1.26 | +0.26 |

Sets that differ by 85 BPM in reality differ by -1.3 BPM as measured. Chopin's
Nocturne op. 9 no. 2 reads 129. A beatless Brian Eno ambient piece reads 132.
Pendulum's 174 BPM drum and bass reads 87, and Bad Religion's 192 BPM hardcore
reads 96.

This is not AcousticBrainz being inaccurate. It locks onto the right number
often enough to be trusted when it does: Deadmau5's "Strobe" at 128.1 against a
published 128, Kendrick Lamar's "HUMBLE." at 150.2 against 150. What it does is
fold tempo into a single octave, which is what every beat tracker does and what
listeners do when they count a 174 BPM track at 87. Fold the *published* tempos
the same way and the ground truth stops separating too: 120 against 116. A BPM
scalar in [60,200] is not a slow-to-fast axis, so no normalisation curve over it
can be one either.

`onset_rate` is onsets per second, in the same `rhythm` block, from the same
request. It separates the two sets with a Cohen's d near 1.1.

## Decision

**Drop `TempoBPM`. Add `OnsetRate`, and give it the palette's eighth dimension,
`driving`.**

### The signal

`OnsetRate` replaces `TempoBPM` on `domain.AudioFeatures` rather than joining
it. A populated field nothing reads is the same defect as an empty one, and #77's
argument for keeping tempo was that it should earn its place.

`analysis.normalizePace` maps onsets per second onto [0,1], linearly over
`paceFloor` 1.5 to `paceCeiling` 4.5. The observed spread across those 19
recordings runs 0.66 (the Eno) to 5.28 (Burial's "Archangel") with the middle
half between 2.41 and 3.47, so the curve is steep across the middle and
saturates inside the extremes. Widening it to the whole observed range flattens
three quarters of real music into a band a palette cannot show.

The range is the softest number here. It rests on 19 recordings chosen to span
the tempo range rather than sampled from a library, so the raw rate is what
`track_features.onset_rate` stores. Retuning the curve then costs nothing, where
re-fetching would cost a second of MusicBrainz rate limit per cached track.

### Its own colour, not a contribution to an existing one

`driving` (`#31C3B3`) is an eighth dimension rather than a term folded into
`energetic` or `danceable`.

Those two read a single feature each precisely because
[ADR 0018](0018-audio-features.md) had to remove liveness from `energetic` after
averaging a real signal with a structurally zero one halved that dimension
permanently. Pace is missing on the same terms liveness was: a track can be
classified by the high-level endpoint and never analyzed for rhythm, and no
playlist has a pace before anything matches. As its own dimension a missing pace
contributes zero weight to a palette normalized to sum to 1, so the colour simply
does not show and the other seven renormalize. Folded into `energetic` it would
halve energy instead.

Two playlists differing only in rhythm, built from the measured rates above:

| | onset rate | pace | share of palette |
|---|---|---|---|
| slow set | 2.37 | 0.29 | **10.4%** |
| fast set | 3.46 | 0.65 | **20.7%** |

`promptgen.visualLanguage` gains the matching entry, so the dimension drives the
image prompt's look and not only its colour.

### A second request that costs nothing

`rhythm.onset_rate` is on the low-level endpoint; only the high-level one was
called before. The two now go out concurrently from `Client.features` rather
than in sequence.

Measured per track, same candidate ids, cold:

| | median | max |
|---|---|---|
| MusicBrainz search | 0.60s | 0.76s |
| high-level (batched) | 0.90s | 2.36s |
| low-level (batched, 8 ids) | 1.04s | 1.51s |
| low-level (1 id) | 0.67s | 1.12s |

Only the MusicBrainz search is rate limited, at one request per second, and
[ADR 0020](0020-coverage-weighted-features.md) put it on its own goroutine so the
fetch happens inside that second. Sequentially the two fetches are 1.94s and the
consumer becomes the bottleneck, taking the analyzing phase from 1.05s/track to
roughly 1.6s. Concurrently they are 1.04s, and the phase stays on the rate
limit's floor.

That is why the batched form is used rather than one low-level request for the
winning recording, which is 0.67s but has to wait for the high-level answer to
know which id won. It is not needed: on all 24 recordings sampled, the first
candidate carrying low-level data was the same one carrying high-level data,
which follows from high-level being derived from low-level submissions. The
batched call costs 155KB median against 53KB and no wall time.

`Client.eachDocument` is the batch protocol both endpoints share.

### Retiring the rows that predate it

A cached `track_features` row written before this change is `present`, carries
four good classifiers, and has no onset rate. Nothing about it looks wrong, so
the cache would serve it forever and the driving dimension would stay dark on
every playlist already analyzed. Measured on the dev database: 948 rows, 402 of
them matched, **0 with an onset rate**.

[ADR 0024](0024-feature-cache-versioning.md) is the mechanism that retires them,
and the argument for why it is a version stamp rather than a `DELETE` in this
migration.

### Where a missing pace is handled

Three places, because the rhythm data can go missing on its own:

- `meanAudioFeatures` averages the rate over its own count. A track classified
  but not analyzed for rhythm reports zero, and dividing that by the full track
  count reads it as a motionless track rather than as silence about one.
- `BlendFeatures` still does not blend it, which is the exemption #77 asked to
  revisit. Revisited, it stands, and for the reason it was written: the estimator
  answers in [0,1], onsets per second is not that, and a text model has nothing
  to ground a DSP event rate in, so it is not asked for one. At coverage weight
  0.25 a measured 4 onsets/sec would blend to 1, which `normalizePace` floors to
  nothing.
- A failed low-level fetch returns the high-level features unharmed. The
  classifiers are most of the palette and stand on their own.

## Consequences

- Every cover changes. The palette normalizes to sum to 1, so an eighth
  dimension re-weights the seven that were there.
- The whole feature cache is invalidated: 948 rows on the dev database go stale
  at once and refill at the 100-lookup cap, so the first few generations after
  this ships pay the analyzing phase they would have paid cold. Nothing is
  deleted, so a rollback finds its own rows still valid.
- Negative entries start expiring at 90 days, which they never did before,
  because `Fresh` was dead code until this change called it.
- The estimator remains the only source for `instrumentalness` and
  `speechiness`, and is now not a source for one further dimension. A playlist
  nothing matched shows seven colours.
- `frontend/src/features/covers/dimensions.ts` gains `driving`. Its `energetic`
  entry also stops claiming a liveness term ADR 0018 removed.
- BPM is gone from the model. Anything wanting a displayable tempo has to fetch
  it again, and should read the octave folding above before trusting it.
