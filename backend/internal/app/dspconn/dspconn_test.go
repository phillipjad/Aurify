package dspconn

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

type fakeProvider struct {
	refreshed domain.DSPConnection
	changed   bool
	err       error
	calls     int
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
	p.calls++
	if p.err != nil {
		return conn, false, p.err
	}
	if !p.changed {
		return conn, false, nil
	}
	return p.refreshed, true, nil
}

func (p *fakeProvider) ListPlaylists(context.Context, domain.DSPConnection) ([]domain.Playlist, error) {
	return nil, nil
}

func (p *fakeProvider) ListTracks(context.Context, domain.DSPConnection, string) ([]domain.Track, error) {
	return nil, nil
}

type fakeRegistry struct{ provider ports.DSPProvider }

func (r fakeRegistry) Get(domain.DSPPlatform) (ports.DSPProvider, error) { return r.provider, nil }
func (r fakeRegistry) Platforms() []domain.DSPPlatform                   { return nil }

type fakeUsers struct {
	user  *domain.User
	saves int
	err   error
}

var _ ports.UserRepository = (*fakeUsers)(nil)

func (u *fakeUsers) Save(_ context.Context, user *domain.User) error {
	u.saves++
	if u.err != nil {
		return u.err
	}
	u.user = user
	return nil
}
func (u *fakeUsers) Create(context.Context, *domain.User) error { return nil }
func (u *fakeUsers) FindByID(context.Context, string) (*domain.User, error) {
	return u.user, nil
}
func (u *fakeUsers) FindByEmail(context.Context, string) (*domain.User, error) {
	return u.user, nil
}

func userWith(conn domain.DSPConnection) *domain.User {
	return &domain.User{
		ID: "user-1",
		Connections: map[domain.DSPPlatform]domain.DSPConnection{
			domain.PlatformYouTubeMusic: conn,
		},
	}
}

// The point of the whole exercise: a refreshed token is written back, so the
// stored credentials match the ones actually in use. Previously oauth2 refreshed
// in memory and dropped the result, leaving the row stale forever.
func TestResolvePersistsARefreshedConnection(t *testing.T) {
	stale := domain.DSPConnection{
		Platform:     domain.PlatformYouTubeMusic,
		AccessToken:  "old",
		RefreshToken: "rt",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}
	fresh := stale
	fresh.AccessToken = "new"
	fresh.ExpiresAt = time.Now().Add(time.Hour)

	users := &fakeUsers{user: userWith(stale)}
	provider := &fakeProvider{refreshed: fresh, changed: true}

	_, conn, err := NewResolver(users, fakeRegistry{provider: provider}).
		Resolve(context.Background(), "user-1", domain.PlatformYouTubeMusic)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if conn.AccessToken != "new" {
		t.Fatalf("returned access token = %q, want the refreshed one", conn.AccessToken)
	}
	if users.saves != 1 {
		t.Fatalf("saves = %d, want 1", users.saves)
	}
	if stored := users.user.Connections[domain.PlatformYouTubeMusic]; stored.AccessToken != "new" {
		t.Fatalf("stored access token = %q, want it updated", stored.AccessToken)
	}
}

// A token that is still good must not cause a write on every read.
func TestResolveDoesNotWriteWhenNothingChanged(t *testing.T) {
	conn := domain.DSPConnection{
		Platform:    domain.PlatformYouTubeMusic,
		AccessToken: "still-good",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	users := &fakeUsers{user: userWith(conn)}
	provider := &fakeProvider{changed: false}

	if _, _, err := NewResolver(users, fakeRegistry{provider: provider}).
		Resolve(context.Background(), "user-1", domain.PlatformYouTubeMusic); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if provider.calls != 1 {
		t.Fatalf("refresh calls = %d, want 1", provider.calls)
	}
	if users.saves != 0 {
		t.Fatalf("saves = %d, want 0: an unchanged token must not be rewritten", users.saves)
	}
}

// If the write fails the caller must hear about it. Proceeding would spend a
// refresh whose result is lost, which is exactly the behaviour this replaced.
func TestResolveFailsWhenTheRefreshCannotBeStored(t *testing.T) {
	stale := domain.DSPConnection{Platform: domain.PlatformYouTubeMusic, AccessToken: "old", RefreshToken: "rt"}
	fresh := stale
	fresh.AccessToken = "new"

	users := &fakeUsers{user: userWith(stale), err: errors.New("db down")}
	provider := &fakeProvider{refreshed: fresh, changed: true}

	if _, _, err := NewResolver(users, fakeRegistry{provider: provider}).
		Resolve(context.Background(), "user-1", domain.PlatformYouTubeMusic); err == nil {
		t.Fatal("expected a failed save to surface")
	}
}

func TestResolveRejectsAnUnconnectedPlatform(t *testing.T) {
	users := &fakeUsers{user: &domain.User{ID: "user-1", Connections: map[domain.DSPPlatform]domain.DSPConnection{}}}

	_, _, err := NewResolver(users, fakeRegistry{provider: &fakeProvider{}}).
		Resolve(context.Background(), "user-1", domain.PlatformSpotify)
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}
