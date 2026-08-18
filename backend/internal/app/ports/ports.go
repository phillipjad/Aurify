// Package ports declares the interfaces (hexagonal "ports") that the
// application layer depends on. Concrete adapters live under internal/platform
// and internal/storage and are wired together in cmd/api. Defining the
// interfaces here keeps the application layer free of any infrastructure
// imports and makes handlers trivially testable with fakes.
package ports

import (
	"context"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// UserRepository persists Aurify users and their DSP connections.
type UserRepository interface {
	// Save updates an existing user. It deliberately does not carry
	// EmailVerified: that state is owned by the verification flows, and letting
	// a general-purpose save write it means any stale in-memory User could
	// un-verify an account. Use Create to set it when the row is born.
	Save(ctx context.Context, user *domain.User) error
	// Create inserts a new user, including EmailVerified. A federated sign-in
	// needs this: the provider has already vouched for the address, and a new
	// federated account has no password, so leaving it unverified strands it
	// behind a verification flow it can never complete.
	Create(ctx context.Context, user *domain.User) error
	FindByID(ctx context.Context, id string) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
}

// CredentialRepository persists local password credentials. A user with no
// stored credential can only sign in through a federated identity.
type CredentialRepository interface {
	Upsert(ctx context.Context, cred domain.Credential) error
	FindByUser(ctx context.Context, userID string) (domain.Credential, error)
	// SetEmailVerified is the only path that changes verification state; the
	// general user save deliberately cannot (see the UpsertUser query).
	SetEmailVerified(ctx context.Context, userID string, verified bool) error
}

// IdentityRepository persists federated sign-in identities.
type IdentityRepository interface {
	Find(ctx context.Context, provider domain.IdentityProvider, subject string) (domain.Identity, error)
	Upsert(ctx context.Context, identity domain.Identity) error
	ListByUser(ctx context.Context, userID string) ([]domain.Identity, error)
}

// SessionRepository persists sessions and their refresh tokens.
type SessionRepository interface {
	Create(ctx context.Context, session domain.Session, refresh domain.RefreshToken) error
	FindSession(ctx context.Context, id string) (domain.Session, error)
	// FindRefreshToken looks a token up by digest, returning ErrNotFound when it
	// is unknown. A returned token may still be expired or already used; the
	// caller decides, because "already used" is a security event rather than a
	// plain miss.
	FindRefreshToken(ctx context.Context, hash []byte) (domain.RefreshToken, error)
	// Rotate atomically marks the presented token used and stores its
	// replacement. It returns domain.ErrTokenReused when the token had already
	// been consumed, which is the signal to revoke the session.
	Rotate(ctx context.Context, presented []byte, next domain.RefreshToken) error
	Touch(ctx context.Context, sessionID string, at time.Time) error
	Revoke(ctx context.Context, sessionID string, at time.Time) error
	RevokeAllForUser(ctx context.Context, userID string, at time.Time) error
}

// EmailTokenRepository persists single-use, expiring email tokens for address
// verification and password reset.
type EmailTokenRepository interface {
	Create(ctx context.Context, token domain.EmailToken) error
	Find(ctx context.Context, hash []byte) (domain.EmailToken, error)
	// Consume marks a token used, returning domain.ErrTokenInvalid if it was
	// already consumed. The guard lives in SQL so a replayed link cannot win a
	// race against a concurrent request.
	Consume(ctx context.Context, hash []byte, at time.Time) error
	DeleteForUser(ctx context.Context, userID string, purpose domain.EmailTokenPurpose) error
}

// AuthBlockRepository persists permanent authentication lockouts, keyed by the
// (ip, identifier) pair. Entries are never removed by the application; clearing
// one is an operator action (see docs/adr/0012-account-lockout-policy.md).
type AuthBlockRepository interface {
	// IsBlocked reports whether the pair is permanently locked out.
	IsBlocked(ctx context.Context, ip, identifier string) (bool, error)
	Block(ctx context.Context, ip, identifier string, failures int, reason string) error
}

// EmailSender delivers transactional mail. Implementations must not block the
// caller on a slow remote server for long; callers treat a send failure as
// non-fatal where the user can retry (for example, resending a verification).
type EmailSender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// CoverRepository persists covers and their generation runs.
type CoverRepository interface {
	// StartRun claims a playlist for a new generation, returning
	// domain.ErrGenerationInFlight when one is already running for it. This is
	// the only way a run may begin, and implementations must make the check and
	// the claim a single atomic write: one run per cover at a time is a
	// guarantee the pipeline relies on, not an optimisation.
	//
	// Like Save, it upserts on the playlist and writes back the row's id.
	StartRun(ctx context.Context, cover *domain.Cover) error
	// Save records a stage change for a run already claimed by StartRun. It
	// upserts on the playlist, not the id: regenerating updates the cover that
	// already exists. It writes back the row's id, which for a regeneration is
	// the existing one rather than the one it was handed.
	Save(ctx context.Context, cover *domain.Cover) error
	// Touch marks a claimed run as still alive and changes nothing else. It is
	// what stops the stuck-cover sweep from reclaiming a playlist out from
	// under a generation that is simply slow, and taking only the id is what
	// makes it safe to call from a goroutine beside the running pipeline.
	Touch(ctx context.Context, coverID string) error
	// FindByID returns the cover with its current artwork, and no revisions.
	FindByID(ctx context.Context, id string) (*domain.Cover, error)
	// ListByUser returns a user's covers newest-first, one page at a time from
	// the keyset position in after (the zero value starts at the top). An empty
	// status returns every lifecycle state; otherwise it filters to that one.
	ListByUser(
		ctx context.Context,
		userID, status string,
		limit int,
		after domain.CoverCursor,
	) ([]domain.Cover, error)
	// Delete removes a cover the user owns, and every revision with it; it
	// returns domain.ErrNotFound when no such cover exists for that user.
	Delete(ctx context.Context, id, userID string) error

	// SaveRevision records a finished run, assigning an id when it has none.
	SaveRevision(ctx context.Context, revision *domain.CoverRevision) error
	// ListRevisions returns a cover's run history newest-first. Failed runs are
	// excluded unless asked for: they are kept because their error explains the
	// failure to the client and their stored prompt explains it to us, but the
	// history is otherwise about renders that exist.
	ListRevisions(ctx context.Context, coverID string, includeFailed bool) ([]domain.CoverRevision, error)
	// DeleteRevision removes one run from a cover the user owns, returning
	// domain.ErrNotFound when either id does not resolve for that user.
	DeleteRevision(ctx context.Context, coverID, revisionID, userID string) error
}

// DSPProvider is implemented by each platform integration (Spotify, Apple
// Music, YouTube Music). It abstracts the provider-specific OAuth flow and data
// ingestion behind a normalized interface.
type DSPProvider interface {
	Platform() domain.DSPPlatform
	// AuthURL returns the provider's OAuth authorization URL for the given
	// opaque state value.
	AuthURL(state string) string
	// Exchange swaps an authorization code for a connection (tokens + identity).
	Exchange(ctx context.Context, code string) (domain.DSPConnection, error)
	// RefreshConnection renews expired credentials, reporting whether anything
	// changed so the caller can persist the result.
	//
	// It exists because the OAuth clients refresh transparently and then throw the
	// new token away: the stored access token stayed stale forever, and every call
	// past its expiry paid for a refresh whose outcome was discarded. Refreshing
	// up front, where the user id is known, is what makes the write possible.
	RefreshConnection(
		ctx context.Context,
		conn domain.DSPConnection,
	) (updated domain.DSPConnection, changed bool, err error)
	ListPlaylists(ctx context.Context, conn domain.DSPConnection) ([]domain.Playlist, error)
	// GetPlaylist reads one playlist's metadata by id.
	//
	// Listing everything to find one is the alternative, which costs a paginated
	// sweep per call; providers can answer this with a single request.
	GetPlaylist(
		ctx context.Context,
		conn domain.DSPConnection,
		playlistID string,
	) (domain.Playlist, error)
	ListTracks(ctx context.Context, conn domain.DSPConnection, playlistID string) ([]domain.Track, error)
}

// DSPRegistry resolves a DSPProvider by platform.
type DSPRegistry interface {
	Get(platform domain.DSPPlatform) (DSPProvider, error)
	Platforms() []domain.DSPPlatform
}

// LyricsClient fetches plain-text lyrics for a track (Aurify uses lrclib.net).
// An empty string with a nil error means "no lyrics found" and is not an error.
type LyricsClient interface {
	Fetch(ctx context.Context, track domain.Track) (string, error)
}

// LyricsRepository remembers lyric lookups so the same track is fetched from the
// provider once rather than once per playlist per run.
//
// Both methods are deliberately batched. A generation resolves a whole playlist
// at a time, and per-track queries would put hundreds of round trips in front of
// work that needs two.
type LyricsRepository interface {
	// FindMany returns the entries that exist, keyed by track key. Keys with no
	// entry are simply absent; a miss is not an error.
	FindMany(ctx context.Context, keys []string) (map[string]domain.CachedLyrics, error)
	// SaveMany upserts entries, so a negative result can later become a positive
	// one without a separate delete.
	SaveMany(ctx context.Context, entries []domain.CachedLyrics) error
}

// TrackFeatureRepository caches feature lookups, in batches for the same reason
// LyricsRepository does: a generation resolves a playlist at a time.
type TrackFeatureRepository interface {
	FindMany(ctx context.Context, keys []string) (map[string]domain.CachedFeatures, error)
	SaveMany(ctx context.Context, entries []domain.CachedFeatures) error
}

// FeatureSource fills in acoustic features for tracks whose DSP supplied none.
//
// The result is index-aligned with tracks, and a track that could not be matched
// comes back with Present false. There is no error to handle, for the same
// reason lyrics.Resolver has none: features are best effort, and absence is
// already a state the analysis engine understands.
type FeatureSource interface {
	Lookup(ctx context.Context, tracks []domain.Track) []domain.AudioFeatures
}

// FeatureEstimator guesses one playlist's acoustic character from its tracks
// and their lyrics, for when too little of it could be matched to a real
// features source to trust the mean (see docs/adr/0020-coverage-weighted-features.md).
//
// Deliberately playlist-level: the palette consumes only the mean, so estimating
// per track would be a more expensive way to reach the same number. What it
// returns is a guess, which is why domain.PlaylistAnalysis records that it was
// used.
type FeatureEstimator interface {
	// Estimate reads lyrics index-aligned with tracks, as lyrics.Resolver
	// returns them; empty strings are expected and a track with none
	// contributes its artist and title alone.
	Estimate(
		ctx context.Context,
		tracks []domain.Track,
		lyrics []string,
	) (domain.AudioFeatures, error)
}

// PlaylistCoverSetter is a DSPProvider that can write a cover back to the
// playlist it came from.
//
// Separate from DSPProvider because it is not something every platform can do.
// Apple Music's API exposes playlist artwork as a read-only URL with no endpoint
// to set one, so folding this into DSPProvider would give that provider a method
// it could only ever stub. Callers type-assert, and the absence is the answer.
type PlaylistCoverSetter interface {
	SetPlaylistCover(
		ctx context.Context,
		conn domain.DSPConnection,
		playlistID string,
		image domain.GeneratedImage,
	) error
}

// SentimentAnalyzer performs NLP sentiment analysis over lyric text.
type SentimentAnalyzer interface {
	Analyze(ctx context.Context, lyrics string) (domain.Sentiment, error)
}

// AnalysisEngine aggregates per-track features and sentiment into a single
// PlaylistAnalysis, including the weighted color palette.
type AnalysisEngine interface {
	Analyze(
		ctx context.Context,
		playlistID string,
		tracks []domain.Track,
		sentiments []domain.Sentiment,
	) (domain.PlaylistAnalysis, error)
}

// PromptGenerator turns a PlaylistAnalysis into an image-generation prompt. It
// is backed by a text model reached over the OpenAI chat-completions API.
type PromptGenerator interface {
	GeneratePrompt(ctx context.Context, analysis domain.PlaylistAnalysis) (string, error)
}

// ImageGenerator renders a prompt into image bytes.
//
// It returns bytes rather than a URL because image providers return the image
// itself, or a link that expires within the hour. Deciding where it lives is
// ImageStore's job (see docs/adr/0016-generation-providers.md).
type ImageGenerator interface {
	GenerateImage(ctx context.Context, prompt string) (domain.GeneratedImage, error)
}

// ImageStore holds generated cover art, keyed by the revision that produced it
// so each run keeps its own bytes. It is the seam between storing bytes in
// PostgreSQL, which is what happens today, and putting them in a bucket later.
//
// There is no URL in either direction: the store owns where the bytes live, and
// the adapter that knows that is also the one that maps a revision back into the
// domain, so it builds the URL there rather than handing it up the pipeline.
type ImageStore interface {
	// Put stores the image for a revision, replacing any bytes already held.
	Put(ctx context.Context, revisionID string, image domain.GeneratedImage) error
	// Find returns the stored image, or domain.ErrNotFound.
	Find(ctx context.Context, revisionID string) (domain.GeneratedImage, error)
}
