package lrclib

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/lyrics/breaker"
)

// RFC 9110 allows both forms, and nothing guarantees which a server picks.
func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

	for name, tc := range map[string]struct {
		header string
		want   time.Duration
		ok     bool
	}{
		"seconds":      {header: "120", want: 2 * time.Minute, ok: true},
		"http date":    {header: now.Add(90 * time.Second).Format(http.TimeFormat), want: 90 * time.Second, ok: true},
		"empty":        {header: "", ok: false},
		"garbage":      {header: "soon please", ok: false},
		"zero seconds": {header: "0", ok: false},
		"negative":     {header: "-5", ok: false},
		"date in past": {header: now.Add(-time.Hour).Format(http.TimeFormat), ok: false},
	} {
		got, ok := parseRetryAfter(tc.header, now)
		if ok != tc.ok {
			t.Errorf("%s: ok = %v, want %v", name, ok, tc.ok)
			continue
		}
		// HTTP dates carry whole seconds, so compare at that resolution.
		if ok && got.Round(time.Second) != tc.want {
			t.Errorf("%s: duration = %s, want %s", name, got, tc.want)
		}
	}
}

// A throttled response has to reach the breaker as something it can act on,
// rather than as an anonymous error.
func TestFetchSurfacesRetryAfterToTheBreaker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "300")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "test").Fetch(t.Context(), domain.Track{Title: "t", Artists: []string{"a"}})
	if err == nil {
		t.Fatal("expected an error for a 429")
	}

	var retry *breaker.RetryAfter
	if !errors.As(err, &retry) {
		t.Fatalf("err = %v, want a *breaker.RetryAfter", err)
	}
	if retry.After != 5*time.Minute {
		t.Fatalf("After = %s, want 5m", retry.After)
	}
}
