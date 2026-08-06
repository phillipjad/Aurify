// Package generatecover orchestrates the full cover-generation pipeline:
// ingest tracks, analyze lyrics, aggregate + weight features into a palette,
// then drive the prompt and image LLM sidecars.
package generatecover

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/phillipjad/aurify/backend/internal/analysis"
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
	// features and estimator are both optional: nil means the pipeline behaves
	// as it did before either existed.
	features  ports.FeatureSource
	estimator ports.FeatureEstimator

	// running counts in-flight background pipelines so Wait can observe them.
	running sync.WaitGroup
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
	features ports.FeatureSource,
	estimator ports.FeatureEstimator,
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
		features:    features,
		estimator:   estimator,
	}
}

// Handle accepts the generation: it validates the user's DSP connection,
// persists a pending cover, and returns its id while the pipeline runs in the
// background. The client observes the progression by polling GET /covers, which
// it already does for every non-terminal cover (see ADR 0019).
func (h *Handler) Handle(ctx context.Context, cmd Command) (string, error) {
	provider, conn, err := h.connections.Resolve(ctx, cmd.UserID, cmd.Platform)
	if err != nil {
		return "", err
	}

	// The name is what the gallery labels a cover with, and it is read here rather
	// than taken from the request: the API declares it required on the response, so
	// whether a cover is identifiable should not depend on the caller supplying it.
	// Fetched before the first save, so the cover never appears untitled.
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
		Status:       domain.CoverStatusPending,
		CreatedAt:    time.Now().UTC(),
	}
	if err := h.covers.Save(ctx, cover); err != nil {
		return "", err
	}

	// The request context is cancelled the moment the response is written, and
	// the pipeline outlives the response by design. WithoutCancel keeps the
	// context's values while dropping that cancellation.
	bg := context.WithoutCancel(ctx)
	h.running.Add(1)
	go func() {
		defer h.running.Done()
		h.run(bg, provider, conn, cover, cmd)
	}()
	return cover.ID, nil
}

// Wait blocks until every accepted generation has finished. Tests use it to
// observe the pipeline's final state. The server does not wait on shutdown:
// a pipeline takes longer than any termination grace period, so interrupted
// covers are failed by the sweep in cmd/api instead.
func (h *Handler) Wait() { h.running.Wait() }

// run executes the pipeline and records the outcome on the cover row, which is
// the job record. There is no caller to return an error to, so every failure
// ends at h.fail.
func (h *Handler) run(
	ctx context.Context,
	provider ports.DSPProvider,
	conn domain.DSPConnection,
	cover *domain.Cover,
	cmd Command,
) {
	cover.Status = domain.CoverStatusAnalyzing
	if err := h.covers.Save(ctx, cover); err != nil {
		h.fail(ctx, cover, err)
		return
	}

	// 1. Ingest the playlist's tracks (normalized audio features included).
	tracks, err := provider.ListTracks(ctx, conn, cmd.PlaylistID)
	if err != nil {
		h.fail(ctx, cover, err)
		return
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
			h.fail(ctx, cover, serr)
			return
		}
		sentiments[i] = s
	}

	// 3. Fill in acoustic features the DSP did not supply, which on YouTube
	//    Music is all of them. Without this every playlist analyzes to the zero
	//    value and five of the seven palette dimensions score exactly 0, so
	//    every cover comes out the same (see docs/adr/0018-audio-features.md).
	//
	//    Best effort, like lyrics: unmatched tracks keep Present false and the
	//    engine skips them exactly as it does today.
	if h.features != nil {
		for i, f := range h.features.Lookup(ctx, tracks) {
			if f.Present && !tracks[i].Features.Present {
				tracks[i].Features = f
			}
		}
	}

	// 4. Aggregate features + sentiment into a normalized, weighted palette.
	result, err := h.analysis.Analyze(ctx, cmd.PlaylistID, tracks, sentiments)
	if err != nil {
		h.fail(ctx, cover, err)
		return
	}

	// Nothing matched, so the palette would be the constant one. Fall back to a
	// guess from the track titles, which is worse than a measurement and far
	// better than declaring the playlist silent.
	if result.AnalyzedCount == 0 && h.estimator != nil {
		if estimated, eerr := h.estimator.Estimate(ctx, tracks); eerr == nil && estimated.Present {
			result.MeanFeatures = estimated
			result.FeaturesEstimated = true
			result.Palette = analysis.BuildPalette(result.MeanFeatures, result.MeanSentiment)
		}
	}
	cover.Analysis = result
	cover.Status = domain.CoverStatusGenerating
	if err := h.covers.Save(ctx, cover); err != nil {
		h.fail(ctx, cover, err)
		return
	}

	// 5. Analysis -> prompt -> image, then store the bytes. The generator returns
	//    the image itself because that is what image APIs hand back; deciding
	//    where it lives is the store's job.
	prompt, err := h.prompts.GeneratePrompt(ctx, result)
	if err != nil {
		h.fail(ctx, cover, err)
		return
	}
	// Recorded before it is used, so a failure downstream keeps it. Image
	// providers refuse prompts, and the prompt is the only thing that explains
	// why; assigning it after the call discarded exactly the evidence needed.
	cover.Prompt = prompt

	image, err := h.images.GenerateImage(ctx, prompt)
	if err != nil {
		h.fail(ctx, cover, err)
		return
	}
	imageURL, err := h.imageStore.Put(ctx, cover.ID, image)
	if err != nil {
		h.fail(ctx, cover, err)
		return
	}

	cover.ImageURL = imageURL
	cover.Status = domain.CoverStatusReady
	if err := h.covers.Save(ctx, cover); err != nil {
		h.fail(ctx, cover, err)
	}
}

// fail marks the cover as failed and persists the cause. The cover row is the
// only channel back to the user, so the log line is for the operator.
func (h *Handler) fail(ctx context.Context, cover *domain.Cover, cause error) {
	slog.Error("cover generation failed", "cover", cover.ID, "error", cause)
	cover.Status = domain.CoverStatusFailed
	cover.Error = cause.Error()
	_ = h.covers.Save(ctx, cover)
}
