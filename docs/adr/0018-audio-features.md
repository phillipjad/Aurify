# 0018 — Audio features from AcousticBrainz, estimated when absent

- Status: Accepted
- Date: 2026-08-04

## Context

Every YouTube Music cover looked identical, and the image models were not the
reason. `youtube_music.go` sets `AudioFeatures{Present: false}` on every track,
so `meanAudioFeatures` returned the zero value. Through `analysis/weights.go`
that left five of seven palette dimensions at exactly 0, and `melancholic`
outranked `euphoric` for every possible lyric polarity. Every playlist analyzed
to `melancholic=0.76, euphoric=0.24`, byte for byte.

## Decision

**Measured features from AcousticBrainz, with a text-model estimate only when
nothing matched.**

Options were measured rather than assumed:

| Source | Outcome |
|---|---|
| Spotify audio-features | Deprecated |
| ReccoBeats | Needs Spotify track ids. Name search returns 0 results for "Radiohead Creep" and parody videos for "Daft Punk Get Lucky" |
| Essentia, or in-house Go DSP | Both measure real audio well, and both need the audio. The only source is downloading from YouTube against their terms |
| **AcousticBrainz** | **Free, no key, real Essentia analysis. Matched 13/13 tracks across three test playlists** |

The blocker for the audio options is acquisition, not the algorithm. In-house
DSP is genuinely tractable for seven crude dimensions and would avoid a sidecar
entirely; it is not ruled out on merit, only on where the audio would come from.

### What is mapped, and what is not

AcousticBrainz returns 18 classifiers of uneven quality. Only the half that
survived inspection is used:

| `AudioFeatures` | Source |
|---|---|
| `danceability` | `danceability` |
| `acousticness` | `mood_acoustic` |
| `energy` | `mood_aggressive`, `mood_party`, inverse `mood_relaxed` |
| `valence` | `mood_happy`, inverse `mood_sad`, `timbre` |

The genre and voice classifiers are ignored. On a vocal country ballad,
`voice_instrumental` answered "instrumental" at p=0.72 and `gender` answered
"female" at p=0.94; `genre_electronic` called both that track and Radiohead's
"Creep" ambient. A test pins this so nothing starts trusting them quietly.

Each classifier is binary with a probability, so the probability is signed by the
label: "not_danceable at p=0.75" means 0.25, not 0.75.

### Costs, both real

MusicBrainz rate-limits to one request per second and blocks rather than
throttles, so the searches are serialized and cached in `track_features`,
mirroring `lyrics_cache` including negative entries. Uncached tracks are capped
per generation: the palette is a mean, so a sample estimates it about as well,
and an uncapped 194-track playlist would add three minutes to a synchronous
generation. Each run widens the cache.

> Amended by [ADR 0020](0020-coverage-weighted-features.md). The cap is now 100
> rather than 25, the two upstream calls are pipelined, and "a sample estimates
> it about as well" holds only above a coverage floor.

AcousticBrainz stopped collecting in 2022, so recent releases are absent.

### The fallback

When nothing in a playlist matched, the text model already configured for
prompt generation estimates playlist-level features from titles and artists.
`domain.PlaylistAnalysis.FeaturesEstimated` records that this happened.

It is a guess. Measured on `gemma3:4b` it discriminates (`instrumentalness`
spread 0.78 across genres, `valence` 0.35, `energy` 0.33) but called power metal
more acoustic than country and returned an identical `speechiness` for
everything.

> Amended by [ADR 0020](0020-coverage-weighted-features.md). The estimate now
> reads lyrics as well as titles, and it is blended in whenever coverage is thin
> rather than only when nothing matched at all.

## Consequences

- Three genres that produced identical palettes now produce three different ones,
  with 4 to 5 live dimensions instead of 2:

  | Playlist | Before | After |
  |---|---|---|
  | country | `melancholic=0.76 euphoric=0.24` | `euphoric=0.26 melancholic=0.26 danceable=0.25 energetic=0.14 organic=0.09` |
  | power metal | `melancholic=0.76 euphoric=0.24` | `danceable=0.32 energetic=0.26 melancholic=0.25 euphoric=0.17` |
  | electronic | `melancholic=0.76 euphoric=0.24` | `danceable=0.30 euphoric=0.23 melancholic=0.21 energetic=0.18 organic=0.08` |

- **`energetic` stopped averaging in `liveness`.** Nothing reachable measures
  liveness, so it sat at zero and permanently halved that dimension, which let
  `danceable` win every playlist regardless of the music. Averaging a real signal
  with a structural zero is a bug, not a compromise. This was found by comparing
  palettes across genres, not by reading the code.
- `introspective` and `intimate` stay flat, since nothing honest measures
  instrumentalness or speechiness. A known gap rather than an oversight.
- Both sources are best effort. A generation completes with AcousticBrainz
  unreachable, the estimator unconfigured, or the cache unreadable.
- Negative results are cached. This library is full of things that are not songs
  (car repair tutorials, a calculus lecture), and each would otherwise re-pay a
  second of rate limit on every generation.
- The palette is now built from someone else's measurement of the audio. That is
  a weaker claim than measuring it ourselves, and `AnalyzedCount` reports how
  many tracks it actually covered.
