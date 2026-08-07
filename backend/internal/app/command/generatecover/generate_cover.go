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

// heartbeatInterval is how often the analyzing phase touches the cover row.
//
// Against the ten-minute staleness threshold in cmd/api this is a wide margin
// on purpose: it costs two writes on a typical run and it is what lets the
// AcousticBrainz lookup cap be chosen for how long a user will watch a stage
// rather than for how long the sweep will tolerate one.
//
// A var only so a test can shorten it; nothing reassigns it in production.
var heartbeatInterval = time.Minute

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

// Handle accepts the generation and returns the pending cover's id; the
// pipeline runs in the background and reports over SSE (see ADR 0019).
func (h *Handler) Handle(ctx context.Context, cmd Command) (string, error) {
	provider, conn, err := h.connections.Resolve(ctx, cmd.UserID, cmd.Platform)
	if err != nil {
		return "", err
	}

	// Read here rather than taken from the request, so a cover's identity does
	// not depend on the caller, and fetched before the first save so it is never
	// untitled. Best effort: a missing label beats a failed generation.
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

	// The request context dies with the response; the pipeline outlives it.
	bg := context.WithoutCancel(ctx)
	h.running.Add(1)
	go func() {
		defer h.running.Done()
		h.run(bg, provider, conn, cover, cmd)
	}()
	return cover.ID, nil
}

// Wait blocks until every accepted generation has finished, for tests. Shutdown
// does not wait: interrupted covers are failed by the sweep in cmd/api.
func (h *Handler) Wait() { h.running.Wait() }

// run executes the pipeline, recording the outcome on the cover row. No caller
// to return to, so every failure ends at h.fail.
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

	result, err := h.analyze(ctx, provider, conn, cover, cmd)
	if err != nil {
		h.fail(ctx, cover, err)
		return
	}

	cover.Analysis = result
	cover.Status = domain.CoverStatusGenerating
	if err := h.covers.Save(ctx, cover); err != nil {
		h.fail(ctx, cover, err)
		return
	}

	// Analysis -> prompt -> image, then store the bytes. The generator returns
	// the image itself because that is what image APIs hand back; deciding
	// where it lives is the store's job.
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

// analyze produces the playlist's analysis: tracks in, palette out.
//
// It is a function rather than the first half of run because of the heartbeat.
// This phase is the long one, it writes nothing to the cover of its own, and
// stopping the heartbeat has to happen on every exit from it; a defer at this
// scope is what guarantees that, and what guarantees the writer is dead before
// run touches the cover again.
func (h *Handler) analyze(
	ctx context.Context,
	provider ports.DSPProvider,
	conn domain.DSPConnection,
	cover *domain.Cover,
	cmd Command,
) (domain.PlaylistAnalysis, error) {
	defer h.heartbeat(ctx, cover)()

	// 1. Ingest the playlist's tracks (normalized audio features included).
	tracks, err := provider.ListTracks(ctx, conn, cmd.PlaylistID)
	if err != nil {
		return domain.PlaylistAnalysis{}, err
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
			return domain.PlaylistAnalysis{}, serr
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
		return domain.PlaylistAnalysis{}, err
	}

	// 5. Too little of the playlist measured to let the mean speak for it. A
	//    mean over two matched tracks has a standard error near 0.2 on a [0,1]
	//    dimension, so it is blended toward a guess from the tracks and their
	//    lyrics in proportion to how much was actually measured
	//    (see docs/adr/0020-coverage-weighted-features.md). Once the coverage
	//    floors are cleared the estimator is not called at all.
	if h.estimator != nil {
		if w := analysis.CoverageWeight(result.AnalyzedCount, result.TrackCount); w < 1 {
			if estimated, eerr := h.estimator.Estimate(ctx, tracks, lyrics); eerr == nil && estimated.Present {
				result.MeanFeatures = analysis.BlendFeatures(result.MeanFeatures, estimated, w)
				result.FeaturesEstimated = true
				result.Palette = analysis.BuildPalette(result.MeanFeatures, result.MeanSentiment)
			}
		}
	}
	return result, nil
}

// heartbeat re-saves the cover on a timer, returning a stop that blocks until
// the writer has exited.
//
// It exists because the analyzing phase writes nothing of its own: run saves the
// cover as analyzing and does not save again until generating, which leaves
// updated_at frozen across lyric resolution and the rate-limited feature
// lookups. The sweep in cmd/api fails non-terminal covers untouched for ten
// minutes, so without this the ceiling on that phase is a deadline rather than a
// choice (see docs/adr/0019-async-cover-generation.md).
//
// Reads of the cover are safe because the caller writes nothing to it until stop
// has returned. Each save fires the notify trigger, so a watching client simply
// re-receives the snapshot it already has.
func (h *Handler) heartbeat(ctx context.Context, cover *domain.Cover) (stop func()) {
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// A heartbeat that cannot be written is not worth failing a
				// generation over; the sweep is the backstop for that.
				_ = h.covers.Save(ctx, cover)
			}
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}

// fail marks the cover failed and persists the cause; the log is for operators.
func (h *Handler) fail(ctx context.Context, cover *domain.Cover, cause error) {
	slog.Error("cover generation failed", "cover", cover.ID, "error", cause)
	cover.Status = domain.CoverStatusFailed
	cover.Error = cause.Error()
	_ = h.covers.Save(ctx, cover)
}
