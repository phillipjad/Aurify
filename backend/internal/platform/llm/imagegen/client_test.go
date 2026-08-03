package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// jpegBytes stands in for a rendered image. The adapter never parses it, so its
// only requirement is that it survives the round trip byte for byte.
var jpegBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}

func serve(t *testing.T, status int, body string) (*httptest.Server, *string, *generateRequest) {
	t.Helper()
	var path string
	var captured generateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server, &path, &captured
}

func TestRendersAnImage(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"result":  map[string]string{"image": base64.StdEncoding.EncodeToString(jpegBytes)},
		"success": true,
		"errors":  []any{},
	})
	server, path, captured := serve(t, http.StatusOK, string(body))

	client := New(server.URL, "acct-1", "@cf/black-forest-labs/flux-1-schnell", "tok")
	img, err := client.GenerateImage(context.Background(), "a violet field")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}

	if !bytes.Equal(img.Bytes, jpegBytes) {
		t.Errorf("bytes = %x, want %x", img.Bytes, jpegBytes)
	}
	if img.ContentType != "image/jpeg" {
		t.Errorf("content type = %q, want image/jpeg", img.ContentType)
	}
	if want := "/accounts/acct-1/ai/run/@cf/black-forest-labs/flux-1-schnell"; *path != want {
		t.Errorf("posted to %q, want %q", *path, want)
	}
	if captured.Prompt != "a violet field" {
		t.Errorf("prompt = %q", captured.Prompt)
	}
	// Above 8 the model rejects the request; below 4 the image degrades. Either
	// way the neuron cost is what this number controls.
	if captured.Steps != 4 {
		t.Errorf("steps = %d, want 4", captured.Steps)
	}
}

func TestSendsTheToken(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"result":{"image":"`+
			base64.StdEncoding.EncodeToString(jpegBytes)+`"},"success":true}`)
	}))
	defer server.Close()

	if _, err := New(server.URL, "acct-1", "m", "tok").
		GenerateImage(context.Background(), "x"); err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if got != "Bearer tok" {
		t.Errorf("Authorization = %q, want Bearer tok", got)
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
			// Cloudflare returns 200 with success=false often enough that
			// trusting the status line would swallow the failure entirely.
			name:   "a failure inside a 200 is still a failure",
			status: http.StatusOK,
			body:   `{"success":false,"errors":[{"code":10000,"message":"Authentication error"}]}`,
			want:   "Authentication error",
		},
		{
			name:   "the message beats the status code",
			status: http.StatusBadRequest,
			body:   `{"success":false,"errors":[{"code":7003,"message":"Could not route to account"}]}`,
			want:   "Could not route to account",
		},
		{
			name:   "a bare status is still reported",
			status: http.StatusServiceUnavailable,
			body:   `nope`,
			want:   "status 503",
		},
		{
			name:   "success=false with no explanation",
			status: http.StatusOK,
			body:   `{"success":false,"errors":[]}`,
			want:   "failure without an error",
		},
		{
			// Storing undecodable bytes would produce a cover that 200s and
			// renders nothing, which is far harder to diagnose than a failure.
			name:   "malformed base64 fails rather than storing garbage",
			status: http.StatusOK,
			body:   `{"success":true,"result":{"image":"!!!not base64!!!"}}`,
			want:   "decoding the image",
		},
		{
			name:   "an empty image is not a result",
			status: http.StatusOK,
			body:   `{"success":true,"result":{"image":""}}`,
			want:   "no image",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _, _ := serve(t, tc.status, tc.body)
			_, err := New(server.URL, "acct-1", "m", "tok").
				GenerateImage(context.Background(), "x")
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// Missing credentials are a supported mode, not a failure: a checkout with no
// Cloudflare account still has to complete a generation.
func TestMissingCredentialsRenderALocalPlaceholder(t *testing.T) {
	for _, tc := range []struct{ name, account, token string }{
		{"no account", "", "tok"},
		{"no token", "acct-1", ""},
		{"neither", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A live URL proves the network is not reached rather than merely
			// unreachable.
			server, _, _ := serve(t, http.StatusOK, `{"success":true}`)

			img, err := New(server.URL, tc.account, "m", tc.token).
				GenerateImage(context.Background(), "a violet field")
			if err != nil {
				t.Fatalf("GenerateImage: %v", err)
			}
			if img.ContentType != "image/png" {
				t.Errorf("content type = %q, want image/png", img.ContentType)
			}
			if _, err := png.Decode(bytes.NewReader(img.Bytes)); err != nil {
				t.Errorf("the placeholder is not a decodable PNG: %v", err)
			}
		})
	}
}

func TestThePlaceholderVariesWithThePrompt(t *testing.T) {
	first := placeholderImage("a violet field")
	again := placeholderImage("a violet field")
	other := placeholderImage("a copper sunrise")

	if !bytes.Equal(first.Bytes, again.Bytes) {
		t.Error("the same prompt should render the same placeholder")
	}
	// Otherwise every cover in the gallery is identical and a stale one is
	// impossible to spot while iterating.
	if bytes.Equal(first.Bytes, other.Bytes) {
		t.Error("different prompts should render different placeholders")
	}
}
