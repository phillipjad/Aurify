package features

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

func tracks(n int) []domain.Track {
	out := make([]domain.Track, n)
	for i := range out {
		out[i] = domain.Track{Title: fmt.Sprintf("song %d", i), Artists: []string{"an artist"}}
	}
	return out
}

func reply(t *testing.T, status int, content string) (*httptest.Server, *chatRequest) {
	t.Helper()
	var captured chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		body, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": content}}},
		})
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server, &captured
}

// The model wraps its answer in a markdown fence even when asked for a JSON
// object through response_format. Every single call fails without this.
func TestFencedJSONIsParsed(t *testing.T) {
	fenced := "```json\n{\"energy\": 0.7, \"valence\": 0.4, \"danceability\": 0.3, \"acousticness\": 0.6}\n```"
	server, _ := reply(t, http.StatusOK, fenced)

	got, err := New(server.URL, "m", "").Estimate(context.Background(), tracks(3))
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if !got.Present {
		t.Fatal("a parsed estimate should be Present")
	}
	if math.Abs(got.Energy-0.7) > 1e-9 || math.Abs(got.Valence-0.4) > 1e-9 {
		t.Errorf("features = %+v, want energy 0.7 and valence 0.4", got)
	}
}

// The model returns out-of-range numbers often enough that an unclamped value
// would quietly skew a palette.
func TestValuesAreClamped(t *testing.T) {
	server, _ := reply(t, http.StatusOK,
		`{"energy": 1.8, "valence": -0.5, "danceability": 0.5, "acousticness": 0.5}`)

	got, err := New(server.URL, "m", "").Estimate(context.Background(), tracks(2))
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if got.Energy != 1 {
		t.Errorf("energy = %v, want it clamped to 1", got.Energy)
	}
	if got.Valence != 0 {
		t.Errorf("valence = %v, want it clamped to 0", got.Valence)
	}
}

// A zeroed AudioFeatures reported as Present is precisely the bug this change
// exists to fix, so a reply naming nothing usable must fail instead.
func TestAnEmptyEstimateIsAFailure(t *testing.T) {
	for _, content := range []string{`{}`, `{"tempo_bpm": 120}`, `{"nonsense": 1}`} {
		server, _ := reply(t, http.StatusOK, content)
		if _, err := New(server.URL, "m", "").Estimate(context.Background(), tracks(2)); err == nil {
			t.Errorf("%s was accepted as an estimate", content)
		}
	}
}

func TestReportsProviderFailures(t *testing.T) {
	for _, tc := range []struct{ name, content, want string }{
		{"prose instead of JSON", "I think this playlist is quite energetic!", "no JSON object"},
		{"malformed JSON", `{"energy": }`, "parsing the estimate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := reply(t, http.StatusOK, tc.content)
			_, err := New(server.URL, "m", "").Estimate(context.Background(), tracks(2))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// An unconfigured checkout keeps today's behaviour rather than failing.
func TestNoBaseURLIsNotAnError(t *testing.T) {
	got, err := New("", "m", "").Estimate(context.Background(), tracks(2))
	if err != nil {
		t.Fatalf("an unconfigured estimator should not error: %v", err)
	}
	if got.Present {
		t.Error("an unconfigured estimator should report nothing measured")
	}
}

// A 194-track playlist would otherwise spend the context window listing songs,
// to reach a single number per dimension.
func TestTheTrackListIsCapped(t *testing.T) {
	server, captured := reply(t, http.StatusOK, `{"energy":0.5,"valence":0.5,"danceability":0.5,"acousticness":0.5}`)

	if _, err := New(server.URL, "m", "").Estimate(context.Background(), tracks(maxTracksInPrompt+60)); err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	user := captured.Messages[1].Content
	if lines := strings.Count(user, "\n- "); lines > maxTracksInPrompt {
		t.Errorf("listed %d tracks, want at most %d", lines, maxTracksInPrompt)
	}
	if !strings.Contains(user, "and 60 more") {
		t.Errorf("the model should be told how many were omitted:\n%s", user)
	}
}

// Artists are what the model actually reasons from; titles alone are much weaker.
func TestArtistsReachTheModel(t *testing.T) {
	server, captured := reply(t, http.StatusOK, `{"energy":0.5,"valence":0.5,"danceability":0.5,"acousticness":0.5}`)

	_, err := New(server.URL, "m", "").Estimate(context.Background(), []domain.Track{
		{Title: "Nox Lux", Artists: []string{"Myth & Roid"}},
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if user := captured.Messages[1].Content; !strings.Contains(user, "Myth & Roid - Nox Lux") {
		t.Errorf("artist and title should both reach the model:\n%s", user)
	}
}
