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

// jpegBytes stands in for a rendered image. The adapter never parses it beyond
// sniffing the type, so its requirements are that it survives the round trip
// byte for byte and that its magic number is real.
var jpegBytes = append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}, make([]byte, 600)...)

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

func okBody(raw []byte) string {
	b, _ := json.Marshal(map[string]any{
		"data": []map[string]any{{"index": 0, "b64_json": base64.StdEncoding.EncodeToString(raw)}},
	})
	return string(b)
}

func TestRendersAnImage(t *testing.T) {
	server, path, captured := serve(t, http.StatusOK, okBody(jpegBytes))

	client := New(server.URL, "gemini-2.5-flash-image", "tok")
	img, err := client.GenerateImage(context.Background(), "a violet field")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}

	if !bytes.Equal(img.Bytes, jpegBytes) {
		t.Errorf("bytes round-tripped wrong: got %d bytes, want %d", len(img.Bytes), len(jpegBytes))
	}
	if *path != "/images/generations" {
		t.Errorf("posted to %q, want /images/generations", *path)
	}
	if captured.Model != "gemini-2.5-flash-image" {
		t.Errorf("model = %q", captured.Model)
	}
	if captured.Prompt != "a violet field" {
		t.Errorf("prompt = %q", captured.Prompt)
	}
	// Album art is square. Providers that do not take this ignore it.
	if captured.Size != "1024x1024" {
		t.Errorf("size = %q, want 1024x1024", captured.Size)
	}
	// Ignored by Gemini, but the diffusion providers default to 20 and FLUX.1
	// [schnell] rejects more than 4, so sending it keeps them reachable.
	if captured.Steps != 4 {
		t.Errorf("steps = %d, want 4", captured.Steps)
	}
	// OpenAI's own spelling, which Gemini follows. A URL would expire within the
	// hour, and the bytes have to be stored regardless.
	if captured.ResponseFormat != "b64_json" {
		t.Errorf("response_format = %q, want b64_json", captured.ResponseFormat)
	}
}

// The content type is stored and later served verbatim, so getting it from the
// bytes rather than a constant is what keeps a PNG from being served as JPEG
// when a provider or model changes.
func TestContentTypeComesFromTheBytes(t *testing.T) {
	pngBytes := append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 600)...)

	for _, tc := range []struct {
		name string
		raw  []byte
		want string
	}{
		{"jpeg", jpegBytes, "image/jpeg"},
		{"png", pngBytes, "image/png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _, _ := serve(t, http.StatusOK, okBody(tc.raw))
			img, err := New(server.URL, "m", "tok").GenerateImage(context.Background(), "x")
			if err != nil {
				t.Fatalf("GenerateImage: %v", err)
			}
			if img.ContentType != tc.want {
				t.Errorf("content type = %q, want %q", img.ContentType, tc.want)
			}
		})
	}
}

func TestSendsTheKey(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, okBody(jpegBytes))
	}))
	defer server.Close()

	if _, err := New(server.URL, "m", "tok").GenerateImage(context.Background(), "x"); err != nil {
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
			// The provider's message is the only thing that distinguishes a bad
			// key from a bad model from a refused prompt.
			name:   "the error body wins over the status code",
			status: http.StatusUnauthorized,
			body:   `{"error":{"message":"Invalid API key provided"}}`,
			want:   "Invalid API key provided",
		},
		{
			name:   "a content refusal is surfaced as written",
			status: http.StatusBadRequest,
			body:   `{"error":{"message":"Your request was rejected by our safety system"}}`,
			want:   "rejected by our safety system",
		},
		{
			name:   "a bare status is still reported",
			status: http.StatusServiceUnavailable,
			body:   `nope`,
			want:   "status 503",
		},
		{
			name:   "an empty data array is not a result",
			status: http.StatusOK,
			body:   `{"data":[]}`,
			want:   "no image",
		},
		{
			// Storing undecodable bytes would produce a cover that 200s and
			// renders nothing, which is harder to diagnose than a failure.
			name:   "malformed base64 fails rather than storing garbage",
			status: http.StatusOK,
			body:   `{"data":[{"b64_json":"!!!not base64!!!"}]}`,
			want:   "decoding the image",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _, _ := serve(t, tc.status, tc.body)
			_, err := New(server.URL, "m", "tok").GenerateImage(context.Background(), "x")
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// A missing key is a supported mode, not a failure: a checkout with no provider
// account still has to complete a generation.
func TestMissingKeyRendersALocalPlaceholder(t *testing.T) {
	// A live URL proves the network is not reached rather than merely
	// unreachable.
	server, path, _ := serve(t, http.StatusOK, okBody(jpegBytes))

	img, err := New(server.URL, "m", "").GenerateImage(context.Background(), "a violet field")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if *path != "" {
		t.Errorf("the provider was called at %q despite having no key", *path)
	}
	if img.ContentType != "image/png" {
		t.Errorf("content type = %q, want image/png", img.ContentType)
	}
	if _, err := png.Decode(bytes.NewReader(img.Bytes)); err != nil {
		t.Errorf("the placeholder is not a decodable PNG: %v", err)
	}
}

func TestThePlaceholderVariesWithThePrompt(t *testing.T) {
	first := placeholderImage("a violet field")
	again := placeholderImage("a violet field")
	other := placeholderImage("a copper sunrise")

	if !bytes.Equal(first.Bytes, again.Bytes) {
		t.Error("the same prompt should render the same placeholder")
	}
	if bytes.Equal(first.Bytes, other.Bytes) {
		t.Error("different prompts should render different placeholders")
	}
}
