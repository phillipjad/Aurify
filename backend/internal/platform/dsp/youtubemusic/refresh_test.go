package youtubemusic

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp"
)

func refreshProvider(t *testing.T, h http.HandlerFunc) (*Provider, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	p := NewProvider(dsp.OAuthConfig{ClientID: "cid", ClientSecret: "secret"})
	p.httpClient = srv.Client()
	p.endpoint = oauth2.Endpoint{TokenURL: srv.URL + "/token"}
	return p, srv.Close
}

func expiredConnection() domain.DSPConnection {
	return domain.DSPConnection{
		AccessToken:  "stale",
		RefreshToken: "rt",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}
}

// invalid_grant is a revoked refresh token, or one aged out of a project still
// in testing. Retrying never fixes it, and it used to reach the client as a 500.
func TestRefreshConnectionInvalidGrantAsksToReconnect(t *testing.T) {
	p, done := refreshProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`))
	})
	defer done()

	_, _, err := p.RefreshConnection(context.Background(), expiredConnection())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, domain.ErrDSPReauthRequired) {
		t.Errorf("error = %v, want it to ask for a reconnect", err)
	}
}

// A refused token endpoint and an unreachable one are not the same thing. The
// grant may be perfectly good with the network merely down, and sending someone
// through an OAuth round trip over a dropped packet costs them their tokens.
func TestRefreshConnectionTransportFailureIsNotAReconnect(t *testing.T) {
	p := NewProvider(dsp.OAuthConfig{ClientID: "cid"})
	p.endpoint = oauth2.Endpoint{TokenURL: "http://127.0.0.1:1/token"}

	_, _, err := p.RefreshConnection(context.Background(), expiredConnection())
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, domain.ErrDSPReauthRequired) {
		t.Errorf("a network failure was reported as a reconnect: %v", err)
	}
}
