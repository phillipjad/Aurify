package generatecover

import (
	"context"
	"errors"
	"testing"

	"github.com/phillipjad/aurify/backend/internal/app/dspconn"
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

func (fakeImages) GenerateImage(context.Context, string) (string, error) {
	return "https://example.test/cover.png", nil
}

func newHandler(provider *fakeProvider) (*Handler, *fakeCovers) {
	users := &fakeUsers{user: &domain.User{
		ID: "user-1",
		Connections: map[domain.DSPPlatform]domain.DSPConnection{
			domain.PlatformYouTubeMusic: {Platform: domain.PlatformYouTubeMusic, AccessToken: "at"},
		},
	}}
	covers := &fakeCovers{}
	handler := NewHandler(
		dspconn.NewResolver(users, fakeRegistry{provider: provider}),
		covers,
		fakeLyrics{},
		fakeSentiment{},
		fakeAnalysis{},
		fakePrompts{},
		fakeImages{},
	)
	return handler, covers
}

// --- tests ---

// The gallery labels each cover with this, and it used to be saved empty, so
// every tile rendered untitled.
func TestGeneratedCoverCarriesThePlaylistName(t *testing.T) {
	provider := &fakeProvider{playlist: domain.Playlist{ID: "PL1", Name: "Chill Vibes"}}
	handler, covers := newHandler(provider)

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

// The lookup is best effort: a cover without a label is a poor outcome, but
// losing a generation the user asked for because a label could not be fetched is
// a worse one.
func TestGenerationSurvivesAFailedNameLookup(t *testing.T) {
	provider := &fakeProvider{playlistErr: errors.New("provider is unhappy")}
	handler, covers := newHandler(provider)

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
