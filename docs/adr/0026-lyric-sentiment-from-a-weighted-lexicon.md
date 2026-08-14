# 0026 — Lyric sentiment from a weighted lexicon, aggregated as a proportion

- Status: Accepted
- Date: 2026-08-14
- Amends: [0025](0025-palette-from-measured-signals.md) (raises `polarityWeight`,
  which that ADR set from what the old analyzer could see)

## Context

`nlp.Analyzer` had been a `SCAFFOLD:` since it was written: membership in a
24-word list, 12 each way, scored as `(pos-neg)/(pos+neg)` over the matched words
and `(pos+neg)/total` as "subjectivity". No weighting, no negation, no stemming,
so "not happy" scored +1.

[ADR 0025](0025-palette-from-measured-signals.md) gave it a permanent job anyway:
a quarter of the palette's bright/dark axis. That made its provenance the problem
rather than its magnitude.

`lyrics_cache` already holds the raw text for every track ever resolved, so this
is answerable offline. Measured over its 1157 tracks with lyrics:

| | 24-word scaffold | weighted lexicon |
|---|---|---|
| words matched per track, mean | **4.3** | **40.2** |
| words matched per track, median | 3 | 35 |
| share of the song matched | 1.3% | 10.3% |
| tracks it matched nothing in | 255 (22%) | 44 (3.8%) |
| tracks it matched under 5 words in | 769 (66%) | 87 (7.5%) |

A track reading -1.0 was a ratio over three words.

## Decision

### The analyzer is VADER's lexicon, read as a ratio

`github.com/jonreiter/govader`, a port of the reference implementation. Polarity
is `(Positive - Negative) / (Positive + Negative)`: the same ratio the scaffold
computed, over 7520 scored entries with negation and intensifiers applied.
Subjectivity keeps its definition, the share of words the lexicon scores, counted
against the lexicon directly.

Two properties of the library that decided how it is read:

- `Compound` normalizes by `x/sqrt(x²+15)`, which saturates to ±1 on anything
  song-length: over the corpus its mean |x| is 0.79, so only the sign survives.
- `Neutral` is a token count over a total that also holds two valence sums, so
  `1 - Neutral` is not a word share and reads 18.4% where 10.3% was matched.

### The aggregation is a proportion, not a mean

`analysis.meanSentiment` averaged polarity over every track with lyrics.
Per-track polarity is now continuous and centred near zero (corpus quartiles
-0.261 and +0.342), so a mean over 50 tracks collapses toward the population
mean, which is the failure [ADR 0025](0025-palette-from-measured-signals.md)
replaced valence over.

Playlist polarity is now the share of tracks clearly bright less the share
clearly bleak, at ±0.25 taken from those corpus quartiles. Measured on the two
playlists below: **0.929 of the axis as a proportion against 0.463 as a mean.**

### `polarityWeight` goes 0.25 to 0.4

Still a minority share. The key scale is a measurement of the audio; this is a
bag of words that reads Billie Eilish's "when the party's over" as +0.79 because
one chorus line repeats eight times. On the 26 tracks below it commits to a side
on 15 and is right about 12 of them.

## Evidence

Two playlists built from tracks already in `lyrics_cache`, songs about death and
grief against songs about celebration, with every audio feature held identical
and no key measured, so the whole difference is the lyrics. Songs whose title
contains one of the 24 scaffold words are excluded: selecting by title otherwise
leaks the rejected signal's own vocabulary into the test, which is enough to
reverse the result.

| | bleak playlist (14) | joyful playlist (12) | spread |
|---|---|---|---|
| polarity, proportion | -0.429 | +0.500 | **0.929** |
| polarity, plain mean | -0.106 | +0.357 | 0.463 |
| polarity, 24-word scaffold | **+0.214** | +0.583 | 0.369 |
| share of the palette | -0.103 | +0.118 | 0.220 |

The scaffold does not merely separate them less. It puts the bleak playlist on
the **wrong side**, reading it as brighter than neutral.

These are one person's library, heavy on hip-hop, EDM and emo, and the sets are
the author's reading of what the songs are about. Unlike
[ADR 0025](0025-palette-from-measured-signals.md)'s genre sets they are the real
cached texts, so re-deriving them needs only the cache.

Four mutations were run against the finished tests, each restored after:
reverting the aggregation to a plain mean, pointing the table back at the 24-word
values, dropping the lyric term from `brightness`, and raising the threshold so
nothing classifies. All four are caught.

## Consequences

- **No cache invalidation, and no migration.** Sentiment is computed at
  generation time from cached lyric *text* and never stored, so this changes the
  next generation and nothing else. `domain.FeaturesVersion` is for
  AcousticBrainz features and is untouched. This stops being true the moment a
  *score* is cached rather than a text; that is when
  [ADR 0024](0024-feature-cache-versioning.md)'s pattern applies.
- **Every cover with lyrics changes**, from both the analyzer and the weight.
- **`gonum` arrives as an indirect dependency**, because `govader` uses `mat.Sum`
  to add up a slice. It costs 0.25 MB of binary, 18.31 to 18.56 MB.
- **The prompt's sentiment sentence was wrong and is rewritten.** It told the
  model "subjectivity %.2f on a scale of 0 (detached) to 1 (personal)". That
  number was coverage, and it was 1.4%. `domain.Sentiment`'s doc comment carried
  the same claim and is corrected with it.
- **`placeholderPrompt` keeps its ±0.25 thresholds**, re-derived rather than
  inherited: on the new scale 11% of 2000 random 30-track playlists drawn from
  the cache clear it.
- **The sequential per-track pass stays correct.** 168µs for a 780-word song, so
  a 50-track playlist spends about 8ms in the analyzer.
- `domain.Sentiment` is unchanged on the wire. `dto` does not carry it, so there
  is no OpenAPI, `schema.ts` or frontend work.
- The analyzer is a bag of words and reads a repeated chorus as the song's
  message. Beating that needs per-track labels to fit against, which this repo
  does not have.
