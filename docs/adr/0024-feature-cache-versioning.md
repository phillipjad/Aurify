# 0024 — Invalidate the feature cache by version, not by delete

- Status: Accepted
- Date: 2026-08-11
- Amends: [0018](0018-audio-features.md) (its cache gains a version and its
  expiry rule is finally enforced)

## Context

[ADR 0023](0023-pace-from-onset-rate.md) added a field to what a feature lookup
produces. Every row already in `track_features` predates it, and none of them
look wrong: they are `present`, they carry four good classifiers, and their only
defect is a field that did not exist when they were written. The cache is
consulted before the network, so without something to retire them the new
dimension stays dark on every playlist already analyzed. On the dev database
that was 948 rows, 402 of them matched, 0 with an onset rate.

This will happen again. A retuned mapping, a different source, another field:
each one makes every cached row an answer the current code would not have given.

The cache is expensive to rebuild. Every row cost a second of MusicBrainz rate
limit, and `track_features` stores only `track_key`, never the recording id, so
there is no backfill cheaper than a re-fetch.

## Decision

**Stamp each row with the build that produced it, and treat a lower stamp as a
miss.**

`track_features.features_version` (migration `00010`) records the version.
`domain.FeaturesVersion` is what the current build writes.
`CachedFeatures.Fresh` rejects anything lower, `Lookup` treats that as a miss,
and the ordinary `ON CONFLICT DO UPDATE` overwrites the row on the re-fetch.

Bumping the constant is the whole procedure. No migration is needed unless the
schema is changing too, and nothing is deleted, so a rollback finds its own rows
still valid and a bump costs re-fetching rather than data.

### Not a DELETE in the migration

The obvious version is `DELETE FROM track_features` in the migration that
changes the shape. It does not survive a rolling deploy.

Migrations run while the old instances are still serving. Those instances empty
their cache, immediately refill it from live traffic in the old shape, and the
new instances read the result as complete. The window is not narrow: it is
however long the rollout takes, against a cache that fills on every generation.

Invalidation therefore has to be something the old build cannot satisfy. A
version number it does not know to write is exactly that: old code omits the
column, the default supplies 0, and 0 is stale by definition. This is the whole
reason the mechanism is a stamp rather than a deletion.

The SQL lever still exists for when a schema change is what prompts the
re-analysis:

```sql
UPDATE track_features SET features_version = 0;
```

### The repository stamps it, not the caller

`SaveMany` writes `domain.FeaturesVersion` and ignores whatever
`CachedFeatures.Version` the caller set. What produced a row is a fact about the
running build, and a caller able to claim a higher version could make a stale
row look current, which is the one thing that breaks the scheme. The field is
populated on read and ignored on write, and the integration test saves an entry
claiming version 99 to prove the write path discards it.

### It made a dead guard live

`CachedFeatures.Fresh` and `NegativeFeaturesTTL` were written with
[ADR 0018](0018-audio-features.md) and never called. `Lookup` read the cache map
directly, so nothing consulted them: a negative entry was remembered
permanently, and the partial index on `(fetched_at) WHERE NOT present`, added by
migration `00006` to support expiring them, had nothing to serve.

The version check belongs in exactly the same place, so wiring it in meant
calling `Fresh`. Negative entries now expire at 90 days as
`NegativeFeaturesTTL` always said they would. That is a behaviour change nobody
asked for, arriving with this one: roughly 546 rows on the dev database are past
that age and will retry, spread across runs by the 100-lookup cap.

## Measured

Against the dev database and the live APIs, one recording already cached with a
good onset rate:

| | features_version | onset | `Fresh` | lookup |
|---|---|---|---|---|
| before | 0 | 5.18 | **false** | 1.92s, went to the network |
| after | 1 | 5.28 | **true** | 1ms, served from cache |

The row was rejected for its stamp alone, refetched, rewritten, and then hit.

## Consequences

- Shipping [0023](0023-pace-from-onset-rate.md) invalidates the entire feature
  cache at once. It refills at the 100-lookup cap, so the first generations
  after it lands pay an analyzing phase they would have paid cold.
- `features_version` is a single number for the whole row, so any change to any
  part of a lookup retires all of it. Splitting it per source would save
  re-fetching the classifiers when only the rhythm changed, at the cost of a
  column per source and a rule per field. Not worth it while one request
  produces the whole row.
- Nothing reconciles the constant with the migrations. Bumping
  `domain.FeaturesVersion` without meaning to silently invalidates the cache,
  and the only thing standing against that is the comment on the constant.
- The same argument applies to `lyrics_cache`, which has the same shape and the
  same problem, and is left alone until something actually changes what a lyric
  lookup stores.
