# 0015 — Lyrics cache, bounded concurrency, and a skip-don't-retry failure policy

- Status: Accepted
- Date: 2026-07-31

## Context

Cover generation fetched lyrics once per track, sequentially, with a 10 second
timeout and no cache. lrclib is a free community service with no documented rate
limit, no `Retry-After` and no `X-RateLimit-*` headers, so there is nothing
upstream to coordinate with.

When lrclib began returning `504 Gateway Timeout`, two generations of 91 and 67
tracks ran for over four minutes without finishing. They would have completed
after roughly fifteen minutes of consecutive timeouts, producing covers whose
palettes were entirely neutral. A healthy run measured about 0.16s per track,
so a 194-track playlist took 31 seconds even when everything worked, and the
same track appearing in two playlists was fetched twice.

## Decision

**Cache lookups in PostgreSQL, batched.** `lyrics_cache` is keyed by a
normalized `artist\ntitle`, deliberately excluding duration so the same song
from different sources shares an entry. Reads and writes are batched, one of
each per generation, because a per-track interface would put hundreds of round
trips in front of work that needs two. Lyrics are immutable, so positive entries
never expire.

**Cache the absences too.** A YouTube Music library is full of titles that will
never match, and without a negative entry each costs a request on every run
forever. Negative entries expire after 30 days, since a track missing today may
be contributed later.

**A memory tier in front, bounded by bytes.** Entry counts cannot express a
value that ranges from hundreds of bytes to tens of kilobytes. Entries are held
compressed with stdlib `compress/flate`, which both stretches the budget and
makes the accounting honest.

**Skip rather than retry.** A circuit breaker opens after five consecutive
faults and answers "no lyrics" instantly for a cool-off that grows to a cap.
Retrying adds load exactly when the service can least take it, and a lyric is a
best-effort signal the pipeline already treats as optional, so skipping is a
legitimate answer where for most resources it would not be. `Retry-After` is
honoured if it ever appears, but only when longer than our own cool-off.

**Bounded concurrency of four** over the misses, and a per-track timeout cut
from 10s to 4s.

## Consequences

- A repeat generation of a playlist makes no outbound lyric requests at all.
- With the provider failing, a large playlist finishes in about 20 seconds
  rather than fifteen minutes, and the cover still completes.
- A failed lookup is never cached, so an outage cannot be remembered as "this
  library has no lyrics".
- The breaker is process-wide, so one user's generation can open it for another.
  That is intended: the thing being protected is the provider, not the request.
- Compression was measured rather than assumed. On real lyrics averaging 1.1KB,
  stdlib flate reached 2.73x while zstd managed 2.54x and lz4 1.82x: at that
  size an input has too little internal history for a stronger algorithm to
  exploit, so no third-party codec was worth a dependency. A preset dictionary
  doubles the ratio to 5.5x and the standard library supports one, but it is not
  used: the budget is already far larger than the data, and a shipped dictionary
  would embed real lyric text in the binary, which is a copyright problem. It
  would have to be built at runtime and frozen, with a per-entry marker for
  anything compressed before it existed.
- The `lyrics` column is `TEXT COMPRESSION lz4` rather than a compressed
  `BYTEA`. PostgreSQL only compresses once a row passes the ~2KB TOAST
  threshold, so short lyrics stay inline uncompressed. That is accepted in
  exchange for a column that stays readable in `psql` and available to full-text
  search later.
