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

// heartbeatInterval is how often a run stamps its cover row as still alive.
//
// It sets the sweep's staleness threshold in cmd/api, which has to clear the
// longest gap a *live* run can leave between writes. That used to be the
// generating phase, which wrote nothing across a 60s prompt call and a 120s
// image call, and it is why the threshold was ten minutes. With every phase
// beating, the threshold is a small multiple of this instead, and a run that
// dies holds its playlist for a fraction as long.
//
// A var only so a test can shorten it; nothing reassigns it in production.
var heartbeatInterval = 30 * time.Second

// maxRunDuration is the ceiling on one generation.
//
// A backstop, not a policy: every upstream call already has its own timeout, and
// the worst legitimate run is around nine minutes if every one of them runs to
// its ceiling. What this catches is the case the heartbeat cannot, because the
// heartbeat is what keeps it alive — a pipeline that never returns holds its
// playlist's claim against the sweep indefinitely.
//
// A var only so a test can shorten it; nothing reassigns it in production.
var maxRunDuration = 15 * time.Minute

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

// Handle accepts the generation and returns the cover's id; the pipeline runs in
// the background and reports over SSE (see ADR 0019).
//
// The id is the *playlist's* cover, so regenerating returns the same id it
// returned last time rather than minting a second tile. The claim upserts on
// (user, platform, playlist) and writes the row's own id back.
//
// One run at a time per cover. StartRun refuses while the cover is non-terminal
// and returns domain.ErrGenerationInFlight, so a double-click, a second tab and
// a second API instance all lose the race in the database rather than each
// starting a pipeline. Two overlapping runs would interleave their status writes
// on one row, and the one that finished second could leave the cover
// non-terminal until the stuck sweep cleared it.
//
// The claim is released by reaching a terminal status, which every exit from run
// does, or by the sweep in cmd/api if the process dies first (ADR 0019). A run
// killed mid-flight therefore holds its playlist until the sweep clears it.
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
	// Nothing may start a pipeline except a claim that succeeded.
	if err := h.covers.StartRun(ctx, cover); err != nil {
		return "", err
	}

	// The request context dies with the response; the pipeline outlives it, but
	// not indefinitely — an unbounded run would hold this playlist's claim for
	// as long as the process lived.
	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), maxRunDuration)
	h.running.Add(1)
	go func() {
		defer h.running.Done()
		defer cancel()
		h.run(bg, provider, conn, cover, cmd)
	}()
	return cover.ID, nil
}

// Wait blocks until every accepted generation has finished, for tests. Shutdown
// does not wait: interrupted covers are failed by the sweep in cmd/api.
func (h *Handler) Wait() { h.running.Wait() }

// run executes the pipeline, recording the run as a revision and its outcome on
// the cover row. No caller to return to, so every failure ends at h.fail.
//
// What the run produces accumulates on rev, not on cover: the cover's prompt,
// artwork and analysis are read from its newest successful revision, so writing
// this run's part-finished output onto the cover is what would blank the tile
// mid-regeneration.
func (h *Handler) run(
	ctx context.Context,
	provider ports.DSPProvider,
	conn domain.DSPConnection,
	cover *domain.Cover,
	cmd Command,
) {
	// Held across the whole pipeline, not just the long phase. Every stage can
	// go quiet for longer than the sweep should have to tolerate — generating
	// spans a prompt call and an image call and writes nothing between them —
	// and the claim on this playlist lasts until the run ends, so the row has to
	// keep saying so throughout.
	//
	// Deferred as the catch-all for the paths that return early, and stopped
	// explicitly before the write that ends the run: a defer fires after that
	// write, which leaves the writer alive across it.
	stopHeartbeat := h.heartbeat(ctx, cover.ID)
	defer stopHeartbeat()

	rev := &domain.CoverRevision{CoverID: cover.ID}

	cover.Status = domain.CoverStatusAnalyzing
	if err := h.covers.Save(ctx, cover); err != nil {
		h.fail(ctx, cover, rev, err)
		return
	}

	result, err := h.analyze(ctx, provider, conn, cmd)
	if err != nil {
		h.fail(ctx, cover, rev, err)
		return
	}

	rev.Analysis = result
	cover.Status = domain.CoverStatusGenerating
	if err := h.covers.Save(ctx, cover); err != nil {
		h.fail(ctx, cover, rev, err)
		return
	}

	// Analysis -> prompt -> image, then store the bytes. The generator returns
	// the image itself because that is what image APIs hand back; deciding
	// where it lives is the store's job.
	prompt, err := h.prompts.GeneratePrompt(ctx, result)
	if err != nil {
		h.fail(ctx, cover, rev, err)
		return
	}
	// Recorded before it is used, so a failure downstream keeps it. Image
	// providers refuse prompts, and the prompt is the only thing that explains
	// why; assigning it after the call discarded exactly the evidence needed.
	rev.Prompt = prompt

	image, err := h.images.GenerateImage(ctx, prompt)
	if err != nil {
		h.fail(ctx, cover, rev, err)
		return
	}

	// The revision lands before the bytes do, because the bytes are keyed by its
	// id and cascade with it. Storing them is the last thing that can fail, and
	// h.fail rewrites this same row as failed if it does, so a revision can
	// never sit at "ready" pointing at nothing.
	rev.Status = domain.CoverStatusReady
	if err := h.covers.SaveRevision(ctx, rev); err != nil {
		h.fail(ctx, cover, rev, err)
		return
	}
	if err := h.imageStore.Put(ctx, rev.ID, image); err != nil {
		h.fail(ctx, cover, rev, err)
		return
	}

	// The writer goes first: this is the write that ends the run, and a beat
	// landing after it would keep a finished cover looking alive, and with it
	// the claim on its playlist.
	stopHeartbeat()

	// Last, because this write is the one the notify trigger fires on: by the
	// time a watching client re-reads the cover, the new artwork is already
	// there to be read.
	cover.Status = domain.CoverStatusReady
	if err := h.covers.Save(ctx, cover); err != nil {
		h.fail(ctx, cover, rev, err)
	}
}

// analyze produces the playlist's analysis: tracks in, palette out.
//
// A function rather than the first half of run because it is the phase with the
// most steps and the least to say about the cover; keeping it separate is what
// leaves run readable as the lifecycle it drives.
func (h *Handler) analyze(
	ctx context.Context,
	provider ports.DSPProvider,
	conn domain.DSPConnection,
	cmd Command,
) (domain.PlaylistAnalysis, error) {
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

// heartbeat stamps the cover row on a timer, returning a stop that blocks until
// the writer has exited.
//
// The pipeline's own writes are too far apart to keep a claim alive: analyzing
// spans lyric resolution and the rate-limited feature lookups, generating spans
// a prompt call and an image call, and neither writes anything in between. The
// sweep in cmd/api reclaims a playlist whose cover has gone quiet, so without
// this the ceiling on a phase would be a deadline rather than a choice (see
// docs/adr/0019-async-cover-generation.md).
//
// It takes the id and not the cover, which is what makes it safe to run beside
// the pipeline for the whole generation: the id is fixed once the run is
// claimed, while the cover struct is being mutated stage by stage. Passing the
// struct would be a data race, and a beat could write back a status the run had
// already moved on from.
//
// Each beat bumps updated_at and so fires the notify trigger; a watching client
// re-receives the snapshot it already has, which costs it a parse and a render
// it was doing every second anyway for the stage label.
func (h *Handler) heartbeat(ctx context.Context, coverID string) (stop func()) {
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
				// Re-checked because a select whose cases are both ready picks
				// one at random: a tick pending at the moment stop() closes
				// done wins that toss half the time and beats once more, after
				// the run has finished with the row. stop() blocking on the
				// writer is not enough on its own to make it dead.
				select {
				case <-done:
					return
				default:
				}
				// A beat that cannot be written is not worth failing a
				// generation over; the sweep is the backstop for that.
				_ = h.covers.Touch(ctx, coverID)
			}
		}
	}()
	// Idempotent, so a run can stop the writer at the point it matters and still
	// leave the deferred stop in place to cover every path that returns early.
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-stopped
		})
	}
}

// fail records the run as a failed revision and marks the cover failed; the log
// is for operators.
//
// The revision goes first, and the cover second, so the write that fires the
// notify trigger is the one after the history is complete. A failed run keeps
// whatever prompt and analysis it got as far as, which is the only evidence
// that explains an image-provider refusal — and it leaves the cover's *artwork*
// alone, so the tile still shows the last render that worked.
func (h *Handler) fail(ctx context.Context, cover *domain.Cover, rev *domain.CoverRevision, cause error) {
	slog.Error("cover generation failed", "cover", cover.ID, "error", cause)

	// Detached, because the most likely reason to be here is that ctx is the
	// thing that died: the run hit maxRunDuration, or an upstream call was
	// cancelled with it. Writing the outcome through a cancelled context would
	// fail both writes and leave the cover claimed until the sweep, which is
	// exactly the wedge the cap exists to avoid.
	ctx = context.WithoutCancel(ctx)

	rev.Status = domain.CoverStatusFailed
	rev.Error = cause.Error()
	rev.CompletedAt = time.Now().UTC()
	_ = h.covers.SaveRevision(ctx, rev)

	cover.Status = domain.CoverStatusFailed
	cover.Error = cause.Error()
	_ = h.covers.Save(ctx, cover)
}
