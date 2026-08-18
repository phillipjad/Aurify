package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// The status code is the contract for these, so it is pinned here rather than
// left to whichever route happens to return one. A domain error that falls
// through to the default arm becomes a 500, which reads to the client as "the
// server is broken" rather than as the answer it actually is.
func TestRespondErrorStatusCodes(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{domain.ErrNotFound, http.StatusNotFound},
		{domain.ErrUnauthorized, http.StatusUnauthorized},
		{domain.ErrEmailTaken, http.StatusConflict},
		// One generation at a time per playlist. A second request is refused,
		// and 409 is what says "not now" rather than "that was invalid".
		{domain.ErrGenerationInFlight, http.StatusConflict},
		// Wrapped, since the repositories and command handlers return these
		// through several layers.
		{fmt.Errorf("generatecover: %w", domain.ErrGenerationInFlight), http.StatusConflict},
		// A grant that cannot do what was asked is the user's to fix, not a
		// server fault. This reached the client as a 500 titled "internal error"
		// with Google's raw JSON as the detail.
		{domain.ErrDSPReauthRequired, http.StatusUnprocessableEntity},
		{fmt.Errorf("youtubemusic: status 403: %w", domain.ErrDSPReauthRequired), http.StatusUnprocessableEntity},
	}

	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			rec := httptest.NewRecorder()
			respondError(mux.NewRouteContext(rec, httptest.NewRequest(http.MethodPost, "/", nil)), tc.err)

			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// Whatever the provider said goes to the log, not to the client: a reconnect is
// the only thing the user can act on, and Google's error JSON is not that.
func TestReauthRequiredSaysWhatToDo(t *testing.T) {
	rec := httptest.NewRecorder()
	respondError(
		mux.NewRouteContext(rec, httptest.NewRequest(http.MethodPost, "/", nil)),
		fmt.Errorf(
			"%w: youtubemusic: set playlist cover: status 403: {\"error\":{\"message\":\"Request had insufficient authentication scopes.\"}}",
			domain.ErrDSPReauthRequired,
		),
	)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Reconnect") {
		t.Errorf("body does not tell the user to reconnect: %s", body)
	}
	for _, leak := range []string{"youtubemusic:", "insufficient authentication scopes", "status 403"} {
		if strings.Contains(body, leak) {
			t.Errorf("provider internals leaked to the client (%q): %s", leak, body)
		}
	}
}

// An in-flight generation is a state the user can wait out, so the response has
// to say which playlist state it is complaining about. A bare 409 with no body
// leaves the UI with nothing to show.
func TestGenerationInFlightExplainsItself(t *testing.T) {
	rec := httptest.NewRecorder()
	respondError(
		mux.NewRouteContext(rec, httptest.NewRequest(http.MethodPost, "/", nil)),
		domain.ErrGenerationInFlight,
	)

	if body := rec.Body.String(); body == "" {
		t.Fatal("409 carries no problem details for the client to render")
	}
}
