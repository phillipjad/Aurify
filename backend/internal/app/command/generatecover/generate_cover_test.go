package generatecover

import (
	"context"
	"errors"
	"testing"

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
	)
	return handler, covers, images
}

// --- tests ---

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

	if _, err := handler.Handle(context.Background(), Command{
		UserID:     "user-1",
		Platform:   domain.PlatformYouTubeMusic,
		PlaylistID: "PL1",
	}); err == nil {
		t.Fatal("expected the generation to fail")
	}

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
	}); err == nil {
		t.Fatal("expected the generation to fail")
	}

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

	last := covers.saved[len(covers.saved)-1]
	if last.Status != domain.CoverStatusReady {
		t.Fatalf("status = %q, want the generation to have completed anyway", last.Status)
	}
	if last.PlaylistName != "" {
		t.Fatalf("name = %q, want it left empty when the lookup failed", last.PlaylistName)
	}
}
