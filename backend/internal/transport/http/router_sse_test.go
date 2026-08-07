package http

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/query/getcover"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/transport/http/handlers"
)

// memCovers is a mutable in-memory cover repository, safe for the live-stream
// test where the handler reads while the test writes.
type memCovers struct {
	mu     sync.Mutex
	covers map[string]*domain.Cover
}

func newMemCovers(covers ...*domain.Cover) *memCovers {
	m := &memCovers{covers: make(map[string]*domain.Cover)}
	for _, c := range covers {
		m.covers[c.ID] = c
	}
	return m
}

func (m *memCovers) Save(_ context.Context, cover *domain.Cover) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := *cover
	m.covers[cover.ID] = &copied
	return nil
}

func (m *memCovers) FindByID(_ context.Context, id string) (*domain.Cover, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cover, ok := m.covers[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copied := *cover
	return &copied, nil
}

func (m *memCovers) ListByUser(context.Context, string, string, int, int) ([]domain.Cover, error) {
	return nil, nil
}
func (m *memCovers) Delete(context.Context, string, string) error { return nil }

// setStatus mutates a stored cover the way the pipeline does: status forward,
// updated_at bumped.
func (m *memCovers) setStatus(id string, status domain.CoverStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.covers[id].Status = status
	m.covers[id].UpdatedAt = m.covers[id].UpdatedAt.Add(time.Second)
}

// eventsDeps points the router's cover reads at a test repository, and its
// change signals at the given channel. A nil watch keeps whatever the default
// test router supplies, which is a stream that is never signalled.
func eventsDeps(covers *memCovers, watch handlers.WatchCover) func(*Deps) {
	return func(d *Deps) {
		d.Application.Queries.GetCover = getcover.NewHandler(covers)
		if watch != nil {
			d.WatchCover = watch
		}
	}
}

func testCover(id, userID string, status domain.CoverStatus) *domain.Cover {
	return &domain.Cover{
		ID: id, UserID: userID, Platform: domain.PlatformSpotify,
		PlaylistID: "PL1", PlaylistName: "Chill", Status: status,
		CreatedAt: time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC),
	}
}

func eventsRequest(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/covers/"+knownCoverID+"/events", nil)
	req.AddCookie(&http.Cookie{Name: handlers.NewCookieWriter(false).AccessCookieName(), Value: token})
	return req
}

// A terminal cover answers with exactly one snapshot and a closed stream: the
// status can never change again, so holding the connection open buys nothing.
func TestCoverEventsSendsASnapshotAndClosesWhenTerminal(t *testing.T) {
	covers := newMemCovers(testCover(knownCoverID, "user-1", domain.CoverStatusReady))
	router, token := newTestRouter(t, eventsDeps(covers, nil))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, eventsRequest(token))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: cover\n") {
		t.Errorf("body has no cover event:\n%s", body)
	}
	if !strings.Contains(body, `"status":"ready"`) {
		t.Errorf("snapshot does not carry the terminal status:\n%s", body)
	}
}

// EventSource authenticates by cookie because it cannot set headers; without
// one the stream must be refused, not opened empty.
func TestCoverEventsRequiresAuthentication(t *testing.T) {
	covers := newMemCovers(testCover(knownCoverID, "user-1", domain.CoverStatusReady))
	router, _ := newTestRouter(t, eventsDeps(covers, nil))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/covers/"+knownCoverID+"/events", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// Someone else's cover id gets the same 404 a plain GET gives, before any
// stream opens.
func TestCoverEventsHidesOtherUsersCovers(t *testing.T) {
	covers := newMemCovers(testCover(knownCoverID, "user-2", domain.CoverStatusReady))
	router, token := newTestRouter(t, eventsDeps(covers, nil))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, eventsRequest(token))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}

// The live path: a subscriber signal makes the stream re-read the cover and
// push the new state, and a terminal state ends the stream. This is the whole
// point of the endpoint — no client polling between the snapshot and ready.
func TestCoverEventsPushesUpdatesUntilTerminal(t *testing.T) {
	covers := newMemCovers(testCover(knownCoverID, "user-1", domain.CoverStatusGenerating))
	signal := make(chan struct{}, 1)
	watch := func(string) (<-chan struct{}, func()) { return signal, func() {} }
	router, token := newTestRouter(t, eventsDeps(covers, watch))

	server := httptest.NewServer(router)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/covers/"+knownCoverID+"/events", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: handlers.NewCookieWriter(false).AccessCookieName(), Value: token})
	res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	reader := bufio.NewReader(res.Body)
	// One SSE event = lines up to a blank line.
	readEvent := func() string {
		var event strings.Builder
		for {
			line, rerr := reader.ReadString('\n')
			event.WriteString(line)
			if rerr != nil || line == "\n" {
				return event.String()
			}
		}
	}

	if first := readEvent(); !strings.Contains(first, `"status":"generating"`) {
		t.Fatalf("first event is not the current snapshot:\n%s", first)
	}

	covers.setStatus(knownCoverID, domain.CoverStatusReady)
	signal <- struct{}{}

	if second := readEvent(); !strings.Contains(second, `"status":"ready"`) {
		t.Fatalf("update was not pushed:\n%s", second)
	}

	// Terminal, so the server must close; a read that blocks here would mean
	// the stream outlives the job.
	if _, err := reader.ReadString('\n'); err == nil {
		t.Fatal("stream still open after the cover became terminal")
	}
}
