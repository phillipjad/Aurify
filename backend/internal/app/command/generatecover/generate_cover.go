// Package generatecover orchestrates the full cover-generation pipeline:
// ingest tracks, analyze lyrics, aggregate + weight features into a palette,
// then drive the prompt and image LLM sidecars.
package generatecover

import (
	"context"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/dspconn"
	"github.com/phillipjad/aurify/backend/internal/app/lyrics"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Command requests a cover for one of a user's playlists.
type Command struct {
	UserID     string
	Platform   domain.DSPPlatform
	PlaylistID string
}

// Handler executes the GenerateCover command.
type Handler struct {
	connections *dspconn.Resolver
	covers      ports.CoverRepository
	lyrics      *lyrics.Resolver
	sentiment   ports.SentimentAnalyzer
	analysis    ports.AnalysisEngine
	prompts     ports.PromptGenerator
	images      ports.ImageGenerator
	imageStore  ports.ImageStore
}

// NewHandler constructs a GenerateCover handler with all of its dependencies.
func NewHandler(
	connections *dspconn.Resolver,
	covers ports.CoverRepository,
	lyricsResolver *lyrics.Resolver,
	sentiment ports.SentimentAnalyzer,
	analysis ports.AnalysisEngine,
	prompts ports.PromptGenerator,
	images ports.ImageGenerator,
	imageStore ports.ImageStore,
) *Handler {
	return &Handler{
		connections: connections,
		covers:      covers,
		lyrics:      lyricsResolver,
		sentiment:   sentiment,
		analysis:    analysis,
		prompts:     prompts,
		images:      images,
		imageStore:  imageStore,
	}
}

// Handle runs the pipeline synchronously and returns the id of the persisted
// cover.
//
// NOTE: A production implementation should enqueue this work and return a
// pending cover immediately, since analysis + generation can take many seconds
// for large playlists. The synchronous flow here keeps the scaffold readable.
func (h *Handler) Handle(ctx context.Context, cmd Command) (string, error) {
	provider, conn, err := h.connections.Resolve(ctx, cmd.UserID, cmd.Platform)
	if err != nil {
		return "", err
	}

	// The name is what the gallery labels a cover with, and it is read here rather
	// than taken from the request: the API declares it required on the response, so
	// whether a cover is identifiable should not depend on the caller supplying it.
	//
	// Best effort on purpose. A cover with no label is a poor outcome; failing a
	// generation the user asked for because a label could not be fetched is a worse
	// one.
	playlistName := ""
	if playlist, perr := provider.GetPlaylist(ctx, conn, cmd.PlaylistID); perr == nil {
		playlistName = playlist.Name
	}

	cover := &domain.Cover{
		UserID:       cmd.UserID,
		Platform:     cmd.Platform,
		PlaylistID:   cmd.PlaylistID,
		PlaylistName: playlistName,
		Status:       domain.CoverStatusAnalyzing,
		CreatedAt:    time.Now().UTC(),
	}
	if err := h.covers.Save(ctx, cover); err != nil {
		return "", err
	}

	// 1. Ingest the playlist's tracks (normalized audio features included).
	tracks, err := provider.ListTracks(ctx, conn, cmd.PlaylistID)
	if err != nil {
		return cover.ID, h.fail(ctx, cover, err)
	}

	// 2. Lyric sentiment per track. Lyrics are best-effort: a missing lyric set
	//    contributes a neutral sentiment rather than failing the whole job.
	//
	//    The resolver handles caching, batching and bounded concurrency, and
	//    never fails, so what comes back is index-aligned with tracks with empty
	//    strings where nothing was found. The sentiment pass stays sequential:
	//    it is local CPU work measured in microseconds, not a network call.
	lyrics := h.lyrics.Resolve(ctx, tracks)
	sentiments := make([]domain.Sentiment, len(tracks))
	for i, text := range lyrics {
		s, serr := h.sentiment.Analyze(ctx, text)
		if serr != nil {
			return cover.ID, h.fail(ctx, cover, serr)
		}
		sentiments[i] = s
	}

	// 3. Aggregate features + sentiment into a normalized, weighted palette.
	result, err := h.analysis.Analyze(ctx, cmd.PlaylistID, tracks, sentiments)
	if err != nil {
		return cover.ID, h.fail(ctx, cover, err)
	}
	cover.Analysis = result
	cover.Status = domain.CoverStatusGenerating
	if err := h.covers.Save(ctx, cover); err != nil {
		return cover.ID, err
	}

	// 4. Analysis -> prompt -> image, then store the bytes. The generator returns
	//    the image itself because that is what image APIs hand back; deciding
	//    where it lives is the store's job.
	prompt, err := h.prompts.GeneratePrompt(ctx, result)
	if err != nil {
		return cover.ID, h.fail(ctx, cover, err)
	}
	// Recorded before it is used, so a failure downstream keeps it. Image
	// providers reject prompts (Workers AI answers a safety refusal for some
	// perfectly ordinary ones), and the prompt is the only thing that explains
	// why; assigning it after the call discarded exactly the evidence needed.
	cover.Prompt = prompt

	image, err := h.images.GenerateImage(ctx, prompt)
	if err != nil {
		return cover.ID, h.fail(ctx, cover, err)
	}
	imageURL, err := h.imageStore.Put(ctx, cover.ID, image)
	if err != nil {
		return cover.ID, h.fail(ctx, cover, err)
	}

	cover.ImageURL = imageURL
	cover.Status = domain.CoverStatusReady
	if err := h.covers.Save(ctx, cover); err != nil {
		return cover.ID, err
	}
	return cover.ID, nil
}

// fail marks the cover as failed, persists the cause, and returns it.
func (h *Handler) fail(ctx context.Context, cover *domain.Cover, cause error) error {
	cover.Status = domain.CoverStatusFailed
	cover.Error = cause.Error()
	_ = h.covers.Save(ctx, cover)
	return cause
}
