package generatecover

import (
	"context"
	"errors"
	"testing"

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
	return []domain.Track{{ID: "v1", Platform: domain.PlatformYouTubeMusic, Title: "Yellow"}}, nil
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
// rather than only its final state.
type fakeCovers struct{ saved []domain.Cover }

var _ ports.CoverRepository = (*fakeCovers)(nil)

func (c *fakeCovers) Save(_ context.Context, cover *domain.Cover) error {
	if cover.ID == "" {
		cover.ID = "cover-1"
	}
	c.saved = append(c.saved, *cover)
	return nil
}
func (c *fakeCovers) FindByID(context.Context, string) (*domain.Cover, error) { return nil, nil }
func (c *fakeCovers) ListByUser(context.Context, string, string, int, int) ([]domain.Cover, error) {
	return nil, nil
}
func (c *fakeCovers) Delete(context.Context, string, string) error { return nil }

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
// the generator produced are the bytes that got stored.
type fakeImageStore struct {
	coverID string
	image   domain.GeneratedImage
}

func (s *fakeImageStore) Put(
	_ context.Context,
	coverID string,
	image domain.GeneratedImage,
) (string, error) {
	s.coverID = coverID
	s.image = image
	return "/api/v1/covers/" + coverID + "/image", nil
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

// The generator hands back bytes and the store decides where they live, so the
// cover's ImageURL has to come from the store rather than from the generator.
// Getting this wrong produces covers that reach "ready" pointing at nothing.
func TestTheStoredImageIsWhatTheCoverPointsAt(t *testing.T) {
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

	if images.coverID != id {
		t.Errorf("stored under %q, want the cover's own id %q", images.coverID, id)
	}
	if string(images.image.Bytes) != "\x89PNG-ish" {
		t.Errorf("stored bytes = %q, want what the generator produced", images.image.Bytes)
	}
	if images.image.ContentType != "image/png" {
		t.Errorf("stored content type = %q, want the generator's", images.image.ContentType)
	}

	last := covers.saved[len(covers.saved)-1]
	if want := "/api/v1/covers/" + id + "/image"; last.ImageURL != want {
		t.Errorf("cover ImageURL = %q, want the store's URL %q", last.ImageURL, want)
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
	if last.Prompt != "a prompt" {
		t.Errorf("prompt = %q, want it kept so the refusal can be diagnosed", last.Prompt)
	}
}

type refusingImages struct{}

func (refusingImages) GenerateImage(context.Context, string) (domain.GeneratedImage, error) {
	return domain.GeneratedImage{}, errors.New("imagegen: Input prompt contains NSFW content")
}

// A cover that reaches "ready" with no retrievable image is worse than a visible
// failure, because the gallery renders it as a permanently broken tile.
func TestAFailedStoreFailsTheCover(t *testing.T) {
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
}

type failingImageStore struct{}

func (failingImageStore) Put(context.Context, string, domain.GeneratedImage) (string, error) {
	return "", errors.New("disk is unhappy")
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
	calls    int
}

func (f *fakeFeatures) Lookup(_ context.Context, tracks []domain.Track) []domain.AudioFeatures {
	f.calls++
	out := make([]domain.AudioFeatures, len(tracks))
	for i := range out {
		out[i] = f.features
	}
	return out
}

type fakeEstimator struct {
	features domain.AudioFeatures
	err      error
	calls    int
}

func (e *fakeEstimator) Estimate(context.Context, []domain.Track) (domain.AudioFeatures, error) {
	e.calls++
	return e.features, e.err
}

// withFeatures builds a handler using the real analysis engine, so AnalyzedCount
// and the palette are computed rather than stubbed. That is the whole point:
// these tests are about what reaches the palette.
func withFeatures(src ports.FeatureSource, est ports.FeatureEstimator) (*Handler, *fakeCovers) {
	users := &fakeUsers{user: &domain.User{
		ID: "user-1",
		Connections: map[domain.DSPPlatform]domain.DSPConnection{
			domain.PlatformYouTubeMusic: {Platform: domain.PlatformYouTubeMusic, AccessToken: "at"},
		},
	}}
	covers := &fakeCovers{}
	handler := NewHandler(
		dspconn.NewResolver(users, fakeRegistry{provider: &fakeProvider{}}),
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

	got := covers.saved[len(covers.saved)-1].Analysis
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

	got := covers.saved[len(covers.saved)-1].Analysis
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

	handler, covers := withFeatures(src, est)
	generate(t, handler)

	if est.calls != 0 {
		t.Errorf("the estimator ran %d times despite measured features", est.calls)
	}
	if got := covers.saved[len(covers.saved)-1].Analysis; got.MeanFeatures.Energy != 0.7 {
		t.Errorf("energy = %.2f, want the measured 0.70", got.MeanFeatures.Energy)
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

	got := covers.saved[len(covers.saved)-1].Analysis
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
