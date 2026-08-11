package generatecover

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/analysis"
	"github.com/phillipjad/aurify/backend/internal/app/dspconn"
	"github.com/phillipjad/aurify/backend/internal/app/lyrics"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// --- fakes ---

type fakeProvider struct {
	playlist    domain.Playlist
	playlistErr error
	// tracks, when nil, is the single-track playlist most of these tests want.
	tracks []domain.Track
}

var _ ports.DSPProvider = (*fakeProvider)(nil)

func (p *fakeProvider) Platform() domain.DSPPlatform { return domain.PlatformYouTubeMusic }
func (p *fakeProvider) AuthURL(string) string        { return "" }
func (p *fakeProvider) Exchange(context.Context, string) (domain.DSPConnection, error) {
	return domain.DSPConnection{}, nil
}

func (p *fakeProvider) RefreshConnection(
	_ context.Context,
	conn domain.DSPConnection,
) (domain.DSPConnection, bool, error) {
	return conn, false, nil
}

func (p *fakeProvider) GetPlaylist(
	_ context.Context,
	_ domain.DSPConnection,
	_ string,
) (domain.Playlist, error) {
	if p.playlistErr != nil {
		return domain.Playlist{}, p.playlistErr
	}
	return p.playlist, nil
}

func (p *fakeProvider) ListPlaylists(context.Context, domain.DSPConnection) ([]domain.Playlist, error) {
	return []domain.Playlist{p.playlist}, nil
}

func (p *fakeProvider) ListTracks(context.Context, domain.DSPConnection, string) ([]domain.Track, error) {
	if p.tracks != nil {
		return p.tracks, nil
	}
	return []domain.Track{{ID: "v1", Platform: domain.PlatformYouTubeMusic, Title: "Yellow"}}, nil
}

// playlistOf builds a playlist big enough for coverage to mean something.
func playlistOf(n int) []domain.Track {
	out := make([]domain.Track, n)
	for i := range out {
		out[i] = domain.Track{
			ID:       fmt.Sprintf("v%d", i),
			Platform: domain.PlatformYouTubeMusic,
			Title:    fmt.Sprintf("track %d", i),
		}
	}
	return out
}

type fakeRegistry struct{ provider ports.DSPProvider }

func (r fakeRegistry) Get(domain.DSPPlatform) (ports.DSPProvider, error) { return r.provider, nil }
func (r fakeRegistry) Platforms() []domain.DSPPlatform                   { return nil }

type fakeUsers struct{ user *domain.User }

var _ ports.UserRepository = (*fakeUsers)(nil)

func (u *fakeUsers) Save(context.Context, *domain.User) error   { return nil }
func (u *fakeUsers) Create(context.Context, *domain.User) error { return nil }
func (u *fakeUsers) FindByID(context.Context, string) (*domain.User, error) {
	return u.user, nil
}
func (u *fakeUsers) FindByEmail(context.Context, string) (*domain.User, error) {
	return u.user, nil
}

// fakeCovers records every save, so a test can inspect the cover as first written
// rather than only its final state. Revisions are recorded the same way: a run's
// output lands there, not on the cover.
//
// It models the real repository's one-run-per-cover claim, because the pipeline
// depends on it: a fake that accepted every StartRun would let these tests pass
// against a handler that had lost the guarantee. The mutex is real too — the
// pipeline writes from its own goroutine while a test may be starting another.
type fakeCovers struct {
	mu        sync.Mutex
	saved     []domain.Cover
	revisions []domain.CoverRevision
	// touched records heartbeats against the status the run was in when each
	// landed, which is how a test checks that every phase keeps the row alive
	// and not just the one that used to.
	touched []domain.CoverStatus
	// inFlight is the claim. One playlist per fake, which is all these tests use.
	inFlight bool
	// status is the run's current stage, for attributing those heartbeats.
	status domain.CoverStatus
}

var _ ports.CoverRepository = (*fakeCovers)(nil)

func (c *fakeCovers) StartRun(ctx context.Context, cover *domain.Cover) error {
	c.mu.Lock()
	if c.inFlight {
		c.mu.Unlock()
		return domain.ErrGenerationInFlight
	}
	c.inFlight = true
	c.mu.Unlock()
	return c.Save(ctx, cover)
}

// Refuses a cancelled context, as a real database call would. That is what makes
// the run-duration cap testable: the write recording the outcome has to be made
// through a context other than the one that just expired.
func (c *fakeCovers) Save(ctx context.Context, cover *domain.Cover) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cover.ID == "" {
		cover.ID = "cover-1"
	}
	// Reaching a terminal status is what releases the claim.
	if cover.Status.Terminal() {
		c.inFlight = false
	}
	c.status = cover.Status
	c.saved = append(c.saved, *cover)
	return nil
}

func (c *fakeCovers) Touch(_ context.Context, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.touched = append(c.touched, c.status)
	return nil
}

// beatsDuring reports how many heartbeats landed while the run was in a stage.
func (c *fakeCovers) beatsDuring(status domain.CoverStatus) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, s := range c.touched {
		if s == status {
			n++
		}
	}
	return n
}
func (c *fakeCovers) FindByID(context.Context, string) (*domain.Cover, error) { return nil, nil }
func (c *fakeCovers) ListByUser(
	context.Context, string, string, int, domain.CoverCursor,
) ([]domain.Cover, error) {
	return nil, nil
}
func (c *fakeCovers) Delete(context.Context, string, string) error { return nil }

// Mints an id like the real repository does, since the pipeline keys the stored
// image bytes by it.
func (c *fakeCovers) SaveRevision(ctx context.Context, rev *domain.CoverRevision) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if rev.ID == "" {
		rev.ID = "revision-1"
	}
	c.revisions = append(c.revisions, *rev)
	return nil
}

func (c *fakeCovers) ListRevisions(context.Context, string, bool) ([]domain.CoverRevision, error) {
	return nil, nil
}
func (c *fakeCovers) DeleteRevision(context.Context, string, string, string) error { return nil }

// lastRevision is the run as finally recorded, which is what a terminal
// assertion is about.
func (c *fakeCovers) lastRevision(t *testing.T) domain.CoverRevision {
	t.Helper()
	if len(c.revisions) == 0 {
		t.Fatal("no revision was recorded, so the run left no history")
	}
	return c.revisions[len(c.revisions)-1]
}

type fakeLyrics struct{}

func (fakeLyrics) Fetch(context.Context, domain.Track) (string, error) { return "some words", nil }

// The pipeline now takes a resolver rather than a bare client, since caching and
// batching a whole playlist has to happen above a single-track interface. This
// store keeps nothing, so every lookup reaches fakeLyrics.
type nullLyricsStore struct{}

func (nullLyricsStore) FindMany(context.Context, []string) (map[string]domain.CachedLyrics, error) {
	return map[string]domain.CachedLyrics{}, nil
}
func (nullLyricsStore) SaveMany(context.Context, []domain.CachedLyrics) error { return nil }

type fakeSentiment struct{}

func (fakeSentiment) Analyze(context.Context, string) (domain.Sentiment, error) {
	return domain.Sentiment{}, nil
}

type fakeAnalysis struct{}

func (fakeAnalysis) Analyze(
	_ context.Context,
	playlistID string,
	_ []domain.Track,
	_ []domain.Sentiment,
) (domain.PlaylistAnalysis, error) {
	return domain.PlaylistAnalysis{PlaylistID: playlistID}, nil
}

type fakePrompts struct{}

func (fakePrompts) GeneratePrompt(context.Context, domain.PlaylistAnalysis) (string, error) {
	return "a prompt", nil
}

type fakeImages struct{}

func (fakeImages) GenerateImage(context.Context, string) (domain.GeneratedImage, error) {
	return domain.GeneratedImage{Bytes: []byte("\x89PNG-ish"), ContentType: "image/png"}, nil
}

// fakeImageStore records what it was handed, so a test can check that the bytes
// the generator produced are the bytes that got stored, under the run that
// produced them.
type fakeImageStore struct {
	revisionID string
	image      domain.GeneratedImage
}

func (s *fakeImageStore) Put(
	_ context.Context,
	revisionID string,
	image domain.GeneratedImage,
) error {
	s.revisionID = revisionID
	s.image = image
	return nil
}

func (s *fakeImageStore) Find(context.Context, string) (domain.GeneratedImage, error) {
	return s.image, nil
}

func newHandler(provider *fakeProvider) (*Handler, *fakeCovers, *fakeImageStore) {
	users := &fakeUsers{user: &domain.User{
		ID: "user-1",
		Connections: map[domain.DSPPlatform]domain.DSPConnection{
			domain.PlatformYouTubeMusic: {Platform: domain.PlatformYouTubeMusic, AccessToken: "at"},
		},
	}}
	covers := &fakeCovers{}
	images := &fakeImageStore{}
	handler := NewHandler(
		dspconn.NewResolver(users, fakeRegistry{provider: provider}),
		covers,
		lyrics.NewResolver(nullLyricsStore{}, fakeLyrics{}),
		fakeSentiment{},
		fakeAnalysis{},
		fakePrompts{},
		fakeImages{},
		images,
		nil,
		nil,
	)
	return handler, covers, images
}

// --- tests ---

// The accept persists the cover as pending before any pipeline work runs: the
// id Handle returns is only useful if a poll can find the row immediately, and
// the poll is the client's only progress signal (see ADR 0019).
func TestHandleAcceptsWithAPendingCover(t *testing.T) {
	provider := &fakeProvider{playlist: domain.Playlist{ID: "PL1", Name: "Chill Vibes"}}
	handler, covers, _ := newHandler(provider)

	id, err := handler.Handle(context.Background(), Command{
		UserID:     "user-1",
		Platform:   domain.PlatformYouTubeMusic,
		PlaylistID: "PL1",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	handler.Wait()

	if covers.saved[0].Status != domain.CoverStatusPending {
		t.Errorf("first saved status = %q, want pending before the pipeline starts", covers.saved[0].Status)
	}
	if covers.saved[0].ID != id {
		t.Errorf("returned id %q is not the pending cover's id %q", id, covers.saved[0].ID)
	}
	if last := covers.saved[len(covers.saved)-1]; last.Status != domain.CoverStatusReady {
		t.Errorf("final status = %q, want the pipeline to have completed", last.Status)
	}
}

// One run at a time per cover. The accept has to be refused outright rather
// than starting a second pipeline over the same row: two would interleave their
// status writes, and whichever finished second could leave the cover stuck
// non-terminal until the sweep cleared it.
func TestASecondGenerationIsRefusedWhileOneIsRunning(t *testing.T) {
	provider := &fakeProvider{playlist: domain.Playlist{ID: "PL1", Name: "Chill Vibes"}}
	handler, covers, images := newHandler(provider)
	// Blocks the first pipeline inside the image call, so it is genuinely still
	// in flight when the second request arrives.
	release := make(chan struct{})
	handler.images = blockingImages{release: release}

	cmd := Command{UserID: "user-1", Platform: domain.PlatformYouTubeMusic, PlaylistID: "PL1"}
	if _, err := handler.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("first Handle: %v", err)
	}

	_, err := handler.Handle(context.Background(), cmd)
	if !errors.Is(err, domain.ErrGenerationInFlight) {
		t.Fatalf("second Handle = %v, want ErrGenerationInFlight", err)
	}

	close(release)
	handler.Wait()

	// Exactly one run happened: one revision, one stored image.
	if len(covers.revisions) == 0 {
		t.Fatal("no revision recorded")
	}
	ids := map[string]bool{}
	for _, rev := range covers.revisions {
		ids[rev.ID] = true
	}
	if len(ids) != 1 {
		t.Errorf("%d distinct revisions, want the single run's", len(ids))
	}
	if images.revisionID == "" {
		t.Error("the surviving run stored no image")
	}

	// And once it has finished, the playlist can be generated again.
	if _, err := handler.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("Handle after the first run finished: %v", err)
	}
	handler.Wait()
}

// blockingImages holds a generation open until released, so a test can observe
// the pipeline mid-flight.
type blockingImages struct{ release chan struct{} }

func (b blockingImages) GenerateImage(context.Context, string) (domain.GeneratedImage, error) {
	<-b.release
	return domain.GeneratedImage{Bytes: []byte("\x89PNG-ish"), ContentType: "image/png"}, nil
}

// The gallery labels each cover with this, and it used to be saved empty, so
// every tile rendered untitled.
func TestGeneratedCoverCarriesThePlaylistName(t *testing.T) {
	provider := &fakeProvider{playlist: domain.Playlist{ID: "PL1", Name: "Chill Vibes"}}
	handler, covers, _ := newHandler(provider)

	if _, err := handler.Handle(context.Background(), Command{
		UserID:     "user-1",
		Platform:   domain.PlatformYouTubeMusic,
		PlaylistID: "PL1",
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	handler.Wait()

	if len(covers.saved) == 0 {
		t.Fatal("no cover was saved")
	}
	// Checked on the first write, not just the last: the name has to be there from
	// the moment the cover appears, since it is visible while it is still analyzing.
	if got := covers.saved[0].PlaylistName; got != "Chill Vibes" {
		t.Fatalf("first saved cover name = %q, want %q", got, "Chill Vibes")
	}
	last := covers.saved[len(covers.saved)-1]
	if last.PlaylistName != "Chill Vibes" {
		t.Fatalf("final cover name = %q, want it preserved", last.PlaylistName)
	}
	if last.Status != domain.CoverStatusReady {
		t.Fatalf("final status = %q, want ready", last.Status)
	}
}

// The bytes belong to the run that produced them, not to the playlist: that is
// what lets earlier artwork stay reachable and what keeps a tile showing its
// last good render while the next run is in flight. Storing them under the cover
// id would overwrite the previous run's image on every regeneration.
func TestTheStoredImageIsKeyedByTheRunThatProducedIt(t *testing.T) {
	provider := &fakeProvider{playlist: domain.Playlist{ID: "PL1", Name: "Chill Vibes"}}
	handler, covers, images := newHandler(provider)

	id, err := handler.Handle(context.Background(), Command{
		UserID:     "user-1",
		Platform:   domain.PlatformYouTubeMusic,
		PlaylistID: "PL1",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	handler.Wait()

	rev := covers.lastRevision(t)
	if images.revisionID != rev.ID {
		t.Errorf("stored under %q, want the revision's id %q", images.revisionID, rev.ID)
	}
	if images.revisionID == id {
		t.Errorf("stored under the cover id %q, which a regeneration would overwrite", id)
	}
	if string(images.image.Bytes) != "\x89PNG-ish" {
		t.Errorf("stored bytes = %q, want what the generator produced", images.image.Bytes)
	}
	if images.image.ContentType != "image/png" {
		t.Errorf("stored content type = %q, want the generator's", images.image.ContentType)
	}

	// The revision has to exist before the bytes are keyed to it, or the
	// foreign key rejects them.
	if rev.CoverID != id {
		t.Errorf("revision belongs to %q, want the cover %q", rev.CoverID, id)
	}
	if rev.Status != domain.CoverStatusReady {
		t.Errorf("revision status = %q, want ready", rev.Status)
	}
}

// A run's analysis and prompt belong to the revision. Publishing them on the
// cover mid-run is what would swap a tile's palette to the new run's before it
// has any artwork to go with it.
func TestTheRunsOutputLandsOnTheRevisionNotTheCover(t *testing.T) {
	provider := &fakeProvider{playlist: domain.Playlist{ID: "PL1", Name: "Chill Vibes"}}
	handler, covers, _ := newHandler(provider)

	if _, err := handler.Handle(context.Background(), Command{
		UserID:     "user-1",
		Platform:   domain.PlatformYouTubeMusic,
		PlaylistID: "PL1",
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	handler.Wait()

	if got := covers.lastRevision(t).Prompt; got != "a prompt" {
		t.Errorf("revision prompt = %q, want the generated one", got)
	}
	for i, c := range covers.saved {
		if c.ImageURL != "" || len(c.Analysis.Palette) > 0 {
			t.Errorf("save %d wrote run output onto the cover: %+v", i, c)
		}
	}
}

// When the image provider refuses a prompt, that prompt is the only thing that
// explains the refusal. It used to be assigned after the image call, so a
// failure persisted an empty string and the evidence was gone.
func TestAFailedImageStillRecordsThePrompt(t *testing.T) {
	provider := &fakeProvider{playlist: domain.Playlist{ID: "PL1"}}
	handler, covers, _ := newHandler(provider)
	handler.images = refusingImages{}

	// The accept succeeds; the failure lands on the cover row, the only channel
	// the client watches once the request has returned.
	if _, err := handler.Handle(context.Background(), Command{
		UserID:     "user-1",
		Platform:   domain.PlatformYouTubeMusic,
		PlaylistID: "PL1",
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	handler.Wait()

	last := covers.saved[len(covers.saved)-1]
	if last.Status != domain.CoverStatusFailed {
		t.Errorf("status = %q, want failed", last.Status)
	}
	// On the revision now, which is the whole reason failed runs are recorded
	// at all rather than only counted.
	rev := covers.lastRevision(t)
	if rev.Status != domain.CoverStatusFailed {
		t.Errorf("revision status = %q, want failed", rev.Status)
	}
	if rev.Prompt != "a prompt" {
		t.Errorf("prompt = %q, want it kept so the refusal can be diagnosed", rev.Prompt)
	}
	if rev.Error == "" {
		t.Error("the failed run records no cause")
	}
}

type refusingImages struct{}

func (refusingImages) GenerateImage(context.Context, string) (domain.GeneratedImage, error) {
	return domain.GeneratedImage{}, errors.New("imagegen: Input prompt contains NSFW content")
}

// A cover that reaches "ready" with no retrievable image is worse than a visible
// failure, because the gallery renders it as a permanently broken tile.
//
// The revision is written before the bytes are, so this is also the case where a
// row already recorded as ready has to be walked back: a ready revision is what
// the tile picks as its current artwork, and one pointing at nothing would stick
// there until the user deleted it.
func TestAFailedStoreFailsTheCoverAndItsRevision(t *testing.T) {
	provider := &fakeProvider{playlist: domain.Playlist{ID: "PL1"}}
	handler, covers, _ := newHandler(provider)
	handler.imageStore = failingImageStore{}

	if _, err := handler.Handle(context.Background(), Command{
		UserID:     "user-1",
		Platform:   domain.PlatformYouTubeMusic,
		PlaylistID: "PL1",
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	handler.Wait()

	last := covers.saved[len(covers.saved)-1]
	if last.Status != domain.CoverStatusFailed {
		t.Errorf("status = %q, want failed", last.Status)
	}
	rev := covers.lastRevision(t)
	if rev.Status != domain.CoverStatusFailed {
		t.Errorf("revision status = %q, want the ready row rewritten as failed", rev.Status)
	}
	// Rewritten, not added: the run is one revision however it ended.
	if rev.ID != covers.revisions[0].ID {
		t.Errorf("revision id changed from %q to %q, so the run left two rows",
			covers.revisions[0].ID, rev.ID)
	}
}

type failingImageStore struct{}

func (failingImageStore) Put(context.Context, string, domain.GeneratedImage) error {
	return errors.New("disk is unhappy")
}

func (failingImageStore) Find(context.Context, string) (domain.GeneratedImage, error) {
	return domain.GeneratedImage{}, errors.New("disk is unhappy")
}

// The lookup is best effort: a cover without a label is a poor outcome, but
// losing a generation the user asked for because a label could not be fetched is
// a worse one.
func TestGenerationSurvivesAFailedNameLookup(t *testing.T) {
	provider := &fakeProvider{playlistErr: errors.New("provider is unhappy")}
	handler, covers, _ := newHandler(provider)

	id, err := handler.Handle(context.Background(), Command{
		UserID:     "user-1",
		Platform:   domain.PlatformYouTubeMusic,
		PlaylistID: "PL1",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if id == "" {
		t.Fatal("no cover id returned")
	}
	handler.Wait()

	last := covers.saved[len(covers.saved)-1]
	if last.Status != domain.CoverStatusReady {
		t.Fatalf("status = %q, want the generation to have completed anyway", last.Status)
	}
	if last.PlaylistName != "" {
		t.Fatalf("name = %q, want it left empty when the lookup failed", last.PlaylistName)
	}
}

// --- acoustic features (#68) ---

// fakeFeatures answers with the same features for every track, or nothing.
type fakeFeatures struct {
	features domain.AudioFeatures
	// matched answers for the first n tracks only, for thin coverage. Zero,
	// the useful default, answers for every track.
	matched int
	// delay stands in for the rate-limited AcousticBrainz lookups, which are
	// most of the analyzing phase's wall time and none of its writes.
	delay time.Duration
	calls int
}

func (f *fakeFeatures) Lookup(_ context.Context, tracks []domain.Track) []domain.AudioFeatures {
	f.calls++
	time.Sleep(f.delay)
	out := make([]domain.AudioFeatures, len(tracks))
	for i := range out {
		if f.matched == 0 || i < f.matched {
			out[i] = f.features
		}
	}
	return out
}

type fakeEstimator struct {
	features domain.AudioFeatures
	err      error
	calls    int
	// lyrics records what the pipeline handed over, since reasoning from them
	// is the whole point of this estimator.
	lyrics []string
}

func (e *fakeEstimator) Estimate(
	_ context.Context,
	_ []domain.Track,
	lyrics []string,
) (domain.AudioFeatures, error) {
	e.calls++
	e.lyrics = lyrics
	return e.features, e.err
}

// withFeatures builds a handler using the real analysis engine, so AnalyzedCount
// and the palette are computed rather than stubbed. That is the whole point:
// these tests are about what reaches the palette.
func withFeatures(src ports.FeatureSource, est ports.FeatureEstimator) (*Handler, *fakeCovers) {
	return withTracks(src, est, nil)
}

// withTracks is withFeatures over a playlist of a given size, for the cases
// where the ratio of matched to total is what is under test.
func withTracks(
	src ports.FeatureSource,
	est ports.FeatureEstimator,
	tracks []domain.Track,
) (*Handler, *fakeCovers) {
	users := &fakeUsers{user: &domain.User{
		ID: "user-1",
		Connections: map[domain.DSPPlatform]domain.DSPConnection{
			domain.PlatformYouTubeMusic: {Platform: domain.PlatformYouTubeMusic, AccessToken: "at"},
		},
	}}
	covers := &fakeCovers{}
	handler := NewHandler(
		dspconn.NewResolver(users, fakeRegistry{provider: &fakeProvider{tracks: tracks}}),
		covers,
		lyrics.NewResolver(nullLyricsStore{}, fakeLyrics{}),
		fakeSentiment{},
		analysis.NewEngine(),
		fakePrompts{},
		fakeImages{},
		&fakeImageStore{},
		src, est,
	)
	return handler, covers
}

func generate(t *testing.T, h *Handler) {
	t.Helper()
	if _, err := h.Handle(context.Background(), Command{
		UserID: "user-1", Platform: domain.PlatformYouTubeMusic, PlaylistID: "PL1",
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	h.Wait()
}

func dominant(a domain.PlaylistAnalysis) string {
	if len(a.Palette) == 0 {
		return ""
	}
	return a.Palette[0].Dimension
}

// The bug this change exists to fix: with no features every playlist analyzes to
// the zero value, five of seven dimensions score 0, and melancholic always wins.
func TestWithoutFeaturesThePaletteIsTheConstantOne(t *testing.T) {
	handler, covers := withFeatures(nil, nil)
	generate(t, handler)

	got := covers.lastRevision(t).Analysis
	if d := dominant(got); d != "melancholic" {
		t.Fatalf("dominant dimension = %q, expected the known-constant %q", d, "melancholic")
	}
	if got.AnalyzedCount != 0 {
		t.Errorf("AnalyzedCount = %d, want 0 when no track carries features", got.AnalyzedCount)
	}
}

// Measured features must reach the palette and move it off that constant.
func TestMeasuredFeaturesChangeThePalette(t *testing.T) {
	src := &fakeFeatures{features: domain.AudioFeatures{
		Danceability: 0.9, Energy: 0.85, Valence: 0.8, Acousticness: 0.1, Present: true,
	}}
	handler, covers := withFeatures(src, nil)
	generate(t, handler)

	got := covers.lastRevision(t).Analysis
	if d := dominant(got); d == "melancholic" {
		t.Errorf("an energetic, danceable, happy playlist still came out melancholic: %+v", got.Palette)
	}
	if got.AnalyzedCount == 0 {
		t.Error("AnalyzedCount is 0 although every track was given features")
	}
	if got.FeaturesEstimated {
		t.Error("measured features must not be recorded as estimated")
	}
}

// The estimator is the fallback, not the plan: it must not run when real
// features were found, or a guess would overwrite a measurement.
func TestTheEstimatorIsSkippedWhenFeaturesWereMeasured(t *testing.T) {
	src := &fakeFeatures{features: domain.AudioFeatures{Energy: 0.7, Present: true}}
	est := &fakeEstimator{features: domain.AudioFeatures{Energy: 0.1, Present: true}}

	// Enough tracks to clear both floors, since "measured" now means measured
	// widely enough as well as measured at all.
	handler, covers := withTracks(src, est, playlistOf(16))
	generate(t, handler)

	if est.calls != 0 {
		t.Errorf("the estimator ran %d times despite measured features", est.calls)
	}
	got := covers.lastRevision(t).Analysis
	if math.Abs(got.MeanFeatures.Energy-0.7) > 1e-9 {
		t.Errorf("energy = %v, want the measured 0.70", got.MeanFeatures.Energy)
	}
}

// When nothing matched, the guess is better than declaring the playlist silent.
func TestTheEstimatorFillsInWhenNothingMatched(t *testing.T) {
	src := &fakeFeatures{features: domain.AudioFeatures{Present: false}}
	est := &fakeEstimator{features: domain.AudioFeatures{
		Danceability: 0.9, Energy: 0.85, Valence: 0.8, Present: true,
	}}

	handler, covers := withFeatures(src, est)
	generate(t, handler)

	got := covers.lastRevision(t).Analysis
	if est.calls != 1 {
		t.Errorf("the estimator ran %d times, want 1", est.calls)
	}
	if !got.FeaturesEstimated {
		t.Error("an estimate must be recorded as one, or it is indistinguishable from a measurement")
	}
	if d := dominant(got); d == "melancholic" {
		t.Errorf("the estimate did not reach the palette: %+v", got.Palette)
	}
	// AnalyzedCount counts measured tracks, so an estimate must not inflate it.
	if got.AnalyzedCount != 0 {
		t.Errorf("AnalyzedCount = %d, want 0: nothing was actually measured", got.AnalyzedCount)
	}
}

// The bug #73 exists to fix: a mean over a couple of matched tracks was treated
// as authoritative purely for being non-zero. Two of seven matched has a
// standard error near 0.2, and it produced danceability 0.99 for power metal.
func TestThinCoverageBlendsTowardTheEstimate(t *testing.T) {
	src := &fakeFeatures{
		features: domain.AudioFeatures{Danceability: 1, Present: true},
		matched:  2,
	}
	est := &fakeEstimator{features: domain.AudioFeatures{Danceability: 0, Present: true}}

	handler, covers := withTracks(src, est, playlistOf(7))
	generate(t, handler)

	got := covers.lastRevision(t).Analysis
	if est.calls != 1 {
		t.Fatalf("the estimator ran %d times, want 1 at 2 of 7 matched", est.calls)
	}
	if !got.FeaturesEstimated {
		t.Error("a blended estimate must be recorded as one")
	}
	// 2 of 7 clears neither floor. The count binds first (2 of 8), so the
	// measurement keeps a quarter of the weight and the guess takes the rest.
	if want := 2.0 / 8.0; math.Abs(got.MeanFeatures.Danceability-want) > 1e-9 {
		t.Errorf("danceability = %v, want %v", got.MeanFeatures.Danceability, want)
	}
	// The count still reports what was measured, so the weight stays
	// reconstructible from a stored analysis.
	if got.AnalyzedCount != 2 {
		t.Errorf("AnalyzedCount = %d, want 2", got.AnalyzedCount)
	}
}

// Full coverage of a very short playlist is still a handful of tracks, and
// AcousticBrainz's own per-track error does not average out over three of them.
// A census clears the share floor and is still held back by the count.
func TestAShortPlaylistIsStillBlended(t *testing.T) {
	src := &fakeFeatures{features: domain.AudioFeatures{Danceability: 1, Present: true}}
	est := &fakeEstimator{features: domain.AudioFeatures{Danceability: 0, Present: true}}

	handler, covers := withTracks(src, est, playlistOf(3))
	generate(t, handler)

	if est.calls != 1 {
		t.Fatalf("the estimator ran %d times, want 1 at three matched tracks", est.calls)
	}
	// Every track measured, so the share floor is cleared; 3 of 8 is what binds.
	got := covers.lastRevision(t).Analysis
	if want := 3.0 / 8.0; math.Abs(got.MeanFeatures.Danceability-want) > 1e-9 {
		t.Errorf("danceability = %v, want %v", got.MeanFeatures.Danceability, want)
	}
}

// The pipeline resolves lyrics a step before the estimator runs, so the
// estimator reads them rather than paying for the same network twice.
func TestTheEstimatorReceivesTheResolvedLyrics(t *testing.T) {
	src := &fakeFeatures{features: domain.AudioFeatures{Present: false}}
	est := &fakeEstimator{features: domain.AudioFeatures{Energy: 0.5, Present: true}}

	handler, _ := withTracks(src, est, playlistOf(3))
	generate(t, handler)

	if len(est.lyrics) != 3 {
		t.Fatalf("the estimator got %d lyric entries, want one per track", len(est.lyrics))
	}
	if est.lyrics[0] != "some words" {
		t.Errorf("lyrics[0] = %q, want the resolved text", est.lyrics[0])
	}
}

// The analyzing phase writes nothing of its own, so without a heartbeat
// updated_at is frozen across it and the stuck-cover sweep in cmd/api would
// eventually fail a generation that is still working (ADR 0019).
func TestEveryPhaseKeepsTheRowAlive(t *testing.T) {
	defer func(d time.Duration) { heartbeatInterval = d }(heartbeatInterval)
	heartbeatInterval = time.Millisecond

	// Both long phases stalled: the feature lookup inside analyzing, and the
	// image call inside generating. Generating is the one that used to beat not
	// at all, and it is the phase whose real upstreams are slowest.
	handler, covers := withFeatures(&fakeFeatures{delay: 50 * time.Millisecond}, nil)
	handler.images = slowImages{delay: 50 * time.Millisecond}
	generate(t, handler)

	for _, stage := range []domain.CoverStatus{domain.CoverStatusAnalyzing, domain.CoverStatusGenerating} {
		if n := covers.beatsDuring(stage); n == 0 {
			t.Errorf("no heartbeat during %s, so the sweep would reclaim a run that is simply slow", stage)
		}
	}

	// A beat outliving the run would keep a finished cover looking alive, and
	// with it the claim on that playlist.
	if n := covers.beatsDuring(domain.CoverStatusReady); n > 0 {
		t.Errorf("%d heartbeats after the run finished, want none", n)
	}
	if last := covers.saved[len(covers.saved)-1]; last.Status != domain.CoverStatusReady {
		t.Errorf("status = %q, want ready", last.Status)
	}
}

type slowImages struct{ delay time.Duration }

func (s slowImages) GenerateImage(ctx context.Context, _ string) (domain.GeneratedImage, error) {
	select {
	case <-time.After(s.delay):
		return domain.GeneratedImage{Bytes: []byte("\x89PNG-ish"), ContentType: "image/png"}, nil
	case <-ctx.Done():
		return domain.GeneratedImage{}, ctx.Err()
	}
}

// A run that never returns would hold its playlist's claim for as long as the
// process lived: the heartbeat it relies on to stay alive is exactly what keeps
// the sweep off it. The cap is the only thing that ends one.
func TestARunThatOverrunsIsFailedAndReleasesItsClaim(t *testing.T) {
	defer func(d time.Duration) { maxRunDuration = d }(maxRunDuration)
	maxRunDuration = 20 * time.Millisecond

	provider := &fakeProvider{playlist: domain.Playlist{ID: "PL1", Name: "Chill Vibes"}}
	handler, covers, _ := newHandler(provider)
	// Far longer than the cap, and cancellable, like a real upstream call.
	handler.images = slowImages{delay: time.Minute}

	cmd := Command{UserID: "user-1", Platform: domain.PlatformYouTubeMusic, PlaylistID: "PL1"}
	if _, err := handler.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	handler.Wait()

	// The outcome has to be written through a context that is not the one that
	// just expired, or the cover stays claimed and the cap achieves nothing.
	last := covers.saved[len(covers.saved)-1]
	if last.Status != domain.CoverStatusFailed {
		t.Fatalf("status = %q, want failed", last.Status)
	}
	if last.Error == "" {
		t.Error("an overrun run records no cause")
	}
	if rev := covers.lastRevision(t); rev.Status != domain.CoverStatusFailed {
		t.Errorf("revision status = %q, want failed", rev.Status)
	}

	// Released: the playlist can be generated again straight away.
	handler.images = fakeImages{}
	if _, err := handler.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("Handle after the overrun: %v", err)
	}
	handler.Wait()
}

// Both sources are best effort. Neither failing may cost the user their cover.
func TestAFailingEstimatorStillProducesACover(t *testing.T) {
	src := &fakeFeatures{features: domain.AudioFeatures{Present: false}}
	est := &fakeEstimator{err: errors.New("ollama is not running")}

	handler, covers := withFeatures(src, est)
	generate(t, handler)

	last := covers.saved[len(covers.saved)-1]
	if last.Status != domain.CoverStatusReady {
		t.Errorf("status = %q, want the generation to have completed anyway", last.Status)
	}
	if last.Analysis.FeaturesEstimated {
		t.Error("a failed estimate was recorded as if it had worked")
	}
}
