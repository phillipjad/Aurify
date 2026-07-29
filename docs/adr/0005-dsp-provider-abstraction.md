# 0005 — DSP provider abstraction + normalized model

- Status: Accepted
- Date: 2026-05-30

## Context

Aurify ingests data from multiple, very different music platforms (Spotify,
Apple Music, YouTube Music). They differ in auth (standard OAuth code flow vs.
Apple's MusicKit JWT + user token vs. Google OAuth) and in the data they expose.
No platform we can register with today serves audio features: YouTube Music
never had a first-party endpoint, and Spotify deprecated `/v1/audio-features`
on 2024-11-27 with no replacement, leaving it available only to clients approved
before that date. The analysis pipeline must not care which platform a track
came from.

## Decision

Define a single `ports.DSPProvider` interface (auth URL, code exchange, list
playlists, list tracks) and one implementation per platform under
`internal/platform/dsp/{spotify,applemusic,youtubemusic}`. A `dsp.Registry`
resolves a provider by `domain.DSPPlatform`.

All providers map their data onto a normalized internal model:
`domain.Track` with `domain.AudioFeatures` (values in `[0,1]`, plus a `Present`
flag for platforms that supply no features). Lyric-based signals
(`domain.Sentiment`) carry the palette, since in practice no provider sets
`Present`. The field stays because the flag is what lets the pipeline stay
honest about the difference, and a future source (a local analyzer, a licensed
feed) can fill it without touching callers.

The registry is constructed in `cmd/api` and does **not** import the provider
sub-packages, keeping the dependency graph acyclic.

## Consequences

- New platforms = implement one interface + register it; nothing downstream
  changes.
- The normalized model is the contract; per-platform quirks are confined to the
  adapter.
- YouTube Music is implemented against the Data API v3 (see
  [ADR 0009](0009-youtube-music-via-data-api-v3.md)). Spotify and Apple Music
  are still stubs returning "not implemented".
- The connect flow is shared by every provider and lives in
  `transport/http/handlers/auth.go`: both legs are top-level browser
  navigations, so `login` redirects to the provider carrying a random state
  stored in a flow cookie, and the callback verifies that state and the platform
  before redirecting back to `/playlists`. It reuses the sign-in flow-state
  helpers under its own cookie, so a half-finished sign-in cannot satisfy a DSP
  callback.
- Token storage is plaintext — encrypt `DSPConnection` tokens at rest before
  production.
