// Package ports declares the interfaces (hexagonal "ports") that the
// application layer depends on. Concrete adapters live under internal/platform
// and internal/storage and are wired together in cmd/api. Defining the
// interfaces here keeps the application layer free of any infrastructure
// imports and makes handlers trivially testable with fakes.
package ports

import (
	"context"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// UserRepository persists Aurify users and their DSP connections.
type UserRepository interface {
	Save(ctx context.Context, user *domain.User) error
	FindByID(ctx context.Context, id string) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
}

// CoverRepository persists generated covers.
type CoverRepository interface {
	Save(ctx context.Context, cover *domain.Cover) error
	FindByID(ctx context.Context, id string) (*domain.Cover, error)
	ListByUser(ctx context.Context, userID string, limit, offset int) ([]domain.Cover, error)
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
	ListPlaylists(ctx context.Context, conn domain.DSPConnection) ([]domain.Playlist, error)
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

// SentimentAnalyzer performs NLP sentiment analysis over lyric text.
type SentimentAnalyzer interface {
	Analyze(ctx context.Context, lyrics string) (domain.Sentiment, error)
}

// AnalysisEngine aggregates per-track features and sentiment into a single
// PlaylistAnalysis, including the weighted color palette.
type AnalysisEngine interface {
	Analyze(ctx context.Context, playlistID string, tracks []domain.Track, sentiments []domain.Sentiment) (domain.PlaylistAnalysis, error)
}

// PromptGenerator turns a PlaylistAnalysis into an image-generation prompt. It
// is backed by a local LLM sidecar.
type PromptGenerator interface {
	GeneratePrompt(ctx context.Context, analysis domain.PlaylistAnalysis) (string, error)
}

// ImageGenerator turns a prompt into a stored image and returns its URL. It is
// backed by a local image-generation LLM sidecar.
type ImageGenerator interface {
	GenerateImage(ctx context.Context, prompt string) (imageURL string, err error)
}
