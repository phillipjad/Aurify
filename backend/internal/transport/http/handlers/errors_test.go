package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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
