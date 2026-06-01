# 0009 — YouTube Music via the YouTube Data API v3

- Status: Accepted
- Date: 2026-05-31

## Context

Aurify's first real DSP adapter is YouTube Music. There is **no official YouTube
Music API**. Two options were considered:

1. **YouTube Data API v3** — Google's official, stable API. Its standard OAuth
   2.0 authorization-code *web* flow maps one-to-one onto the existing
   `ports.DSPProvider` interface (see [ADR 0005](0005-dsp-provider-abstraction.md)).
2. **`ytmusicapi`** — an unofficial Python library reverse-engineering the
   internal `music.youtube.com` API. Richer music metadata (real artist/album,
   library, "Liked Music"), but it requires a separate Python sidecar and a
   device-code auth flow that does **not** fit the redirect-based port.

## Decision

Implement `internal/platform/dsp/youtubemusic` against the **YouTube Data API
v3**, using `golang.org/x/oauth2` (+ `/google`) for the OAuth dance and
automatic token refresh, and hand-rolled `net/http` calls for the three read
endpoints — matching the existing `lrclib` adapter style.

- Scope: `https://www.googleapis.com/auth/youtube.readonly`.
- `AuthURL` uses `access_type=offline` + `prompt=consent` to guarantee a refresh
  token.
- Reads: `playlists.list?mine=true`, `playlistItems.list`, and `videos.list`
  (durations are not on playlistItems). Each is paginated; ~5 quota units cover a
  100-track playlist.
- Tracks normalize to `domain.Track` with `AudioFeatures.Present == false`;
  "`<Artist> - Topic`" owner channels yield clean artist names, and
  private/deleted items are skipped.

## Consequences

- No new service to operate; the adapter drops into the existing registry and
  the `youtube_music` value already flows through the OpenAPI contract.
- Metadata is video-centric: no `album`/`isrc`, and the auto-generated "Liked
  Music" playlist is not exposed by the Data API. Lyric sentiment compensates,
  per ADR 0005.
- Adds the `golang.org/x/oauth2` dependency (and transitive
  `cloud.google.com/go/compute/metadata`).
- If richer library/liked-songs access is needed later, a `ytmusicapi` sidecar
  can sit behind the same port without touching callers — revisit in a new ADR.
