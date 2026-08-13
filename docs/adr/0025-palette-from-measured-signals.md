# 0025 — A palette of signals the sources actually measure

- Status: Accepted
- Date: 2026-08-12
- Amends: [0018](0018-audio-features.md) (replaces its mapping table, and
  reverses its rejection of `voice_instrumental`)

## Context

`domain.AudioFeatures` is modelled on Spotify's `/v1/audio-features`. Aurify does
not call Spotify; it calls AcousticBrainz. The palette has been inheriting a
vocabulary from a source it does not have, and the mismatch has been patched one
dimension at a time: liveness removed in [0018](0018-audio-features.md), tempo
replaced in [0023](0023-pace-from-onset-rate.md).

Measured over 87 completed revisions in the dev database
(`cover_revisions.analysis`, `status = 'ready'`), 61 of which carried features:

| dimension | mean share | sd | zero on |
|---|---|---|---|
| danceable | 27.7% | **7.8** | 0% |
| organic | 13.1% | **9.0** | 0% |
| energetic | 12.8% | 5.3 | 0% |
| melancholic | 23.0% | **3.8** | 0% |
| euphoric | 22.0% | **2.5** | 0% |
| introspective | 0.5% | 2.1 | **92%** |
| intimate | 0.4% | 1.5 | **92%** |

Of the eight dimensions: two were dead, two were a fixed tint, and three did the
work. (`driving` is excluded: it existed on 2 revisions.) `euphoric` and
`melancholic` together took 45.0% of every palette while moving by two and four
points across the whole library, and they correlate -0.076, so they were not
redundant with each other. They were both simply flat.

**The cause is averaging.** A bounded per-track feature meaned over 50 tracks
collapses toward the population mean. `danceable` and `organic` survive because
AcousticBrainz's classifiers are near-binary per track, so their mean is a real
proportion. Valence does not, because it is a mean of three classifiers before it
is a mean over tracks.

## Decision

**Seven dimensions, each reading something a source measures.** `intimate` is
dropped, `introspective` is given a real source, and the bright/dark pair is
rebuilt on a signal that survives averaging.

### Bright and dark, from the key rather than from the mood

`tonal.key_scale` is major or minor: a measurement of the audio rather than a
trained classifier, near-binary per track, so its mean over a playlist is the
proportion in a major key. `domain.AudioFeatures.Tonality` carries it signed, +1
major and -1 minor, so that 0 means both "no key was measured" and "evenly
split", which are the same thing to a palette.

Five constructed genre sets, measured against the live API, as the share of the
palette the bright colour takes less the share the dark one takes:

| set | n | major | share | mean valence | share if read from valence |
|---|---|---|---|---|---|
| folk and acoustic | 6 | 0.833 | **+0.307** | 0.502 | +0.003 |
| upbeat pop | 5 | 0.800 | +0.286 | 0.775 | +0.268 |
| electronic and house | 9 | 0.667 | +0.182 | 0.444 | -0.069 |
| jazz standards | 9 | 0.444 | -0.069 | 0.470 | -0.038 |
| metal and dark | 7 | 0.143 | **-0.322** | 0.439 | -0.075 |

Valence separates only the most extreme set and pins the other four into 0.439 to
0.502: set the pop set aside and the remaining four spread over 0.078 of the
palette against the key scale's 0.629. `analysis/tonal_test.go` is that table,
with a companion test that fails if the axis is ever pointed back at valence.

These numbers are the softest thing here. They come from sets assembled by genre
rather than from real playlists, because tracks are only reachable inside the
generation pipeline, and the candidate picker takes the first recording carrying
data, which can be a live or instrumental version of the song. That inflates the
error rather than the signal. They want re-measuring against real playlists with
recording ids pinned up front.

**One axis, two colours, weighted by distance from neutral.** `euphoric` scores
`max(0, brightness)` and `melancholic` scores `max(0, -brightness)`, so only one
is ever lit and its weight is how lopsided the playlist is. A single bright
dimension would have left a minor-key playlist with no dark colour at all and
renormalized it onto four warm ones. A constant pair is what this replaces.

**The lyrics keep a quarter of the axis.** `polarityWeight` is 0.25, set by what
the analyzer can see rather than by a measurement: `nlp.Analyzer` holds 24 words
and matches about 1.4% of a song, so a playlist polarity of 0.8 is a ratio over
roughly four matched words. Across the 77 revisions with lyrics, mean |polarity|
is 0.20 and the maximum is 0.80, so the number does move; it is thin rather than
absent. It keeps a fixed minority share rather than taking the whole axis when no
key was measured, because a cover should not be decided by a word count.
Improving the analyzer is a follow-up and needs no palette change.

### `introspective` gets a source, `intimate` is dropped

[ADR 0018](0018-audio-features.md) rejected `voice_instrumental` on one country
ballad it called instrumental at p=0.72. Over 24 recordings whose vocal status is
not in dispute (14 usable after 503s and missing classifiers) it was right about
71% of them and separated the two groups by 0.442. That 71% is a floor: the
labels are per song and the data is per recording, so an instrumental cover in
the candidate list scores as the song being wrong. Radiohead's "Creep" reading
instrumental at p=1.00 is almost certainly that. It is near-binary per track, so
its mean is the proportion of the playlist with no singing.

The ballad remains a miss and is kept as a test rather than as a memory.

Nothing in the 18 classifiers measures speech content, so `intimate` is deleted
rather than left dark. A dimension that cannot be lit is not a quiet gap; it is a
share of every palette permanently spent on a colour that never appears.

### Both new signals are free

`tonal.key_scale` is in the low-level document [ADR 0023](0023-pace-from-onset-rate.md)
already fetches, and `voice_instrumental` is in the high-level one. Neither costs
a request.

Reading a second field from the low-level document does have one trap:
`eachDocument` stops at the first submission its callback accepts, so two
callbacks would be two independent searches and could take the pace from one
recording and the key from another. `Client.lowLevel` reads both from one
submission in one pass.

`eachDocument` also now iterates a recording's submissions in sorted order rather
than in Go's map order. Observed live before this: the same Burial recording read
5.18 then 5.28 onsets per second on consecutive runs. Pre-existing, and it
matters more once two fields have to agree about which submission they came from.

### The estimator answers on the same axis

The text model is never asked for a key. `BlendFeatures` maps its valence onto
`Tonality` as `2v-1` before anything reads it, including on the path where
nothing matched and the estimate is returned whole. Without that, every playlist
released after AcousticBrainz stopped collecting in 2022 would show neither
bright nor dark. Unlike the pace, this blends: both sides are the same axis on
the same scale.

### Retiring the rows that predate it

`domain.FeaturesVersion` goes 1 to 2, which is the whole invalidation
([ADR 0024](0024-feature-cache-versioning.md)). Migration `00011` adds `tonality`
and `instrumentalness` to `track_features`; both default to 0, which is safe
precisely because 0 is what "nobody measured this" means for each.

## Consequences

- Every cover changes. Seven dimensions renormalize where eight did, and the two
  that were 45% of every palette are now 0% of a playlist with no key and up to
  60% of one that is all in one mode.
- The feature cache invalidates and refills at the 100-lookup cap, so the first
  few generations after this ships pay the analyzing phase they would have paid
  cold. Nothing is deleted, so a rollback finds its own rows still valid.
- `Liveness`, `Loudness` and `Speechiness` are now read by no dimension.
  `Valence` is read on the estimated side only. They are left on the model and in
  the estimator's prompt rather than deleted: `Valence` is the estimator's
  brightness and the comparison the tonal test rests on, and removing the other
  three touches the prompt, the estimate, both aggregates and the JSONB shape for
  no behaviour change. Worth deleting when something else forces that file open.
- The mirror of dimension names in `promptgen.visualLanguage` is now checked
  against the engine's own output rather than against a hand-written copy of it.
  The frontend's copy in `dimensions.ts` is still hand-maintained.
- The key-scale and `voice_instrumental` numbers above both want re-measuring
  against real playlists with pinned recording ids before they are quoted as
  accuracy rather than as evidence of a direction.
- Verifying this build against the live API showed the confound doing exactly
  what it is described as doing. Ten tracks, eight matched: both fields came back
  populated on every one of them, `introspective` took 13.8% and 23.6% of two
  palettes where it was zero on 92% of runs before, and Joy Division's "Love Will
  Tear Us Apart" read major at `voice_instrumental` 0.98. That is a vocal song in
  a minor key, so the candidate picker landed on something else. At four or five
  usable tracks per set that is enough to move a mean, which is the whole reason
  a re-measure has to pin recording ids rather than search by name.
