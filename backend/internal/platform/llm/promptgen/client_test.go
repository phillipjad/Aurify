package promptgen

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

func testAnalysis() domain.PlaylistAnalysis {
	return domain.PlaylistAnalysis{
		PlaylistID: "PL1",
		TrackCount: 12,
		Palette: []domain.ColorWeight{
			{Dimension: "melancholic", HexColor: "#5C4D7D", Weight: 0.6},
			{Dimension: "euphoric", HexColor: "#FFE15D", Weight: 0.4},
		},
		MeanSentiment: domain.Sentiment{Polarity: -0.2, Subjectivity: 0.1, HasLyrics: true},
	}
}

// reply serves one chat-completions response and captures the request that
// produced it.
func reply(t *testing.T, status int, body string) (*httptest.Server, *chatRequest) {
	t.Helper()
	var captured chatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured)
		if r.URL.Path != "/chat/completions" {
			t.Errorf("posted to %q, want /chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server, &captured
}

func TestSendsTheModelAndThePalette(t *testing.T) {
	server, captured := reply(t, http.StatusOK,
		`{"choices":[{"message":{"role":"assistant","content":"a violet field"}}]}`)

	client := New(server.URL, "gemma3:4b", "sk-test")
	prompt, err := client.GeneratePrompt(context.Background(), testAnalysis())
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	if prompt != "a violet field" {
		t.Errorf("prompt = %q", prompt)
	}

	if captured.Model != "gemma3:4b" {
		t.Errorf("model = %q, want gemma3:4b", captured.Model)
	}
	if len(captured.Messages) != 2 || captured.Messages[0].Role != "system" {
		t.Fatalf("messages = %+v, want a system message then a user message", captured.Messages)
	}
	// The palette is the whole input signal; sending a request without it would
	// still look like a working integration.
	user := captured.Messages[1].Content
	for _, want := range []string{"melancholic", "#5C4D7D", "60%", "polarity -0.20"} {
		if !strings.Contains(user, want) {
			t.Errorf("user message is missing %q:\n%s", want, user)
		}
	}
}

func TestSendsTheKeyOnlyWhenThereIsOne(t *testing.T) {
	for _, tc := range []struct {
		name   string
		key    string
		header string
	}{
		{"a key is forwarded", "sk-test", "Bearer sk-test"},
		{"no key sends no header", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Get("Authorization")
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"x"}}]}`)
			}))
			defer server.Close()

			if _, err := New(server.URL, "m", tc.key).
				GeneratePrompt(context.Background(), testAnalysis()); err != nil {
				t.Fatalf("GeneratePrompt: %v", err)
			}
			if got != tc.header {
				t.Errorf("Authorization = %q, want %q", got, tc.header)
			}
		})
	}
}

func TestNoBaseURLUsesThePlaceholder(t *testing.T) {
	prompt, err := New("", "m", "").GeneratePrompt(context.Background(), testAnalysis())
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	if !strings.Contains(prompt, "#5C4D7D") {
		t.Errorf("placeholder should carry the dominant color, got %q", prompt)
	}
}

func TestReportsProviderFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			// The provider's own message is what says which of the two is wrong.
			name:   "the error body wins over the status code",
			status: http.StatusNotFound,
			body:   `{"error":{"message":"model \"nope\" not found"}}`,
			want:   `model "nope" not found`,
		},
		{
			name:   "a bare status is still reported",
			status: http.StatusBadGateway,
			body:   `nope`,
			want:   "status 502",
		},
		{
			name:   "no choices is a failure, not an empty prompt",
			status: http.StatusOK,
			body:   `{"choices":[]}`,
			want:   "no choices",
		},
		{
			// Passing "" on to the image model would render something arbitrary.
			name:   "whitespace-only content is a failure",
			status: http.StatusOK,
			body:   `{"choices":[{"message":{"content":"   \n "}}]}`,
			want:   "empty prompt",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := reply(t, tc.status, tc.body)
			_, err := New(server.URL, "m", "").GeneratePrompt(context.Background(), testAnalysis())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestStripsInlineReasoning(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    string
	}{
		{"a closed block is removed", "<think>hmm, violet?</think>\na violet field", "a violet field"},
		{"content without one is untouched", "a violet field", "a violet field"},
		{"a block mid-string is not a reasoning block", "paint <think> shapes", "paint <think> shapes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(chatResponse{
				Choices: []struct {
					Message chatMessage `json:"message"`
				}{{Message: chatMessage{Content: tc.content}}},
			})
			server, _ := reply(t, http.StatusOK, string(body))

			got, err := New(server.URL, "m", "").GeneratePrompt(context.Background(), testAnalysis())
			if err != nil {
				t.Fatalf("GeneratePrompt: %v", err)
			}
			if got != tc.want {
				t.Errorf("prompt = %q, want %q", got, tc.want)
			}
		})
	}
}

// An unterminated block means the model never reached an answer, so there is
// nothing to hand the image model.
func TestUnterminatedReasoningIsAnError(t *testing.T) {
	server, _ := reply(t, http.StatusOK,
		`{"choices":[{"message":{"content":"<think>still thinking about it"}}]}`)

	if _, err := New(server.URL, "m", "").
		GeneratePrompt(context.Background(), testAnalysis()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestDescribesAPlaylistWithoutLyrics(t *testing.T) {
	a := testAnalysis()
	a.MeanSentiment = domain.Sentiment{HasLyrics: false}

	got := describeAnalysis(a)
	if !strings.Contains(got, "No lyrics were available") {
		t.Errorf("absent lyrics should be stated, not implied by omission:\n%s", got)
	}
	if strings.Contains(got, "polarity") {
		t.Errorf("a neutral zero should not be reported as a measurement:\n%s", got)
	}
}
