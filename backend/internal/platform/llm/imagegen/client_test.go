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

var pngBytes = append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 600)...)

// serve answers one request with the given content type and body, capturing what
// was asked of it.
func serve(t *testing.T, status int, contentType, body string) (*httptest.Server, *string, *generateRequest) {
	t.Helper()
	var path string
	var captured generateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured)
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server, &path, &captured
}

func envelope(raw []byte) string {
	b, _ := json.Marshal(map[string]any{
		"result":  map[string]string{"image": base64.StdEncoding.EncodeToString(raw)},
		"success": true,
		"errors":  []any{},
	})
	return string(b)
}

func TestRendersAnImage(t *testing.T) {
	server, path, captured := serve(t, http.StatusOK, "application/json", envelope(jpegBytes))

	client := New(server.URL, "acct-1", "@cf/leonardoai/lucid-origin", "tok")
	img, err := client.GenerateImage(context.Background(), "a violet field")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}

	if !bytes.Equal(img.Bytes, jpegBytes) {
		t.Errorf("bytes round-tripped wrong: got %d bytes, want %d", len(img.Bytes), len(jpegBytes))
	}
	// The account and model are both path segments assembled here rather than
	// configured, so a wrong join asks the wrong endpoint entirely.
	if want := "/accounts/acct-1/ai/run/@cf/leonardoai/lucid-origin"; *path != want {
		t.Errorf("posted to %q, want %q", *path, want)
	}
	if captured.Prompt != "a violet field" {
		t.Errorf("prompt = %q", captured.Prompt)
	}
	if captured.Width != 1024 || captured.Height != 1024 {
		t.Errorf("size = %dx%d, want 1024x1024", captured.Width, captured.Height)
	}
}

// Workers AI models disagree on how they answer: the newer ones wrap base64 in
// Cloudflare's envelope, the Stable Diffusion ones stream the image itself.
// Trying a different model must not need a code change, since model-shopping is
// exactly what this adapter exists to survive.
func TestHandlesBothResponseShapes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		body        string
		want        []byte
		wantType    string
	}{
		{"base64 in a JSON envelope", "application/json", envelope(jpegBytes), jpegBytes, "image/jpeg"},
		{"a raw image stream", "image/png", string(pngBytes), pngBytes, "image/png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _, _ := serve(t, http.StatusOK, tc.contentType, tc.body)
			img, err := New(server.URL, "acct-1", "m", "tok").GenerateImage(context.Background(), "x")
			if err != nil {
				t.Fatalf("GenerateImage: %v", err)
			}
			if !bytes.Equal(img.Bytes, tc.want) {
				t.Errorf("bytes = %d, want %d", len(img.Bytes), len(tc.want))
			}
			// Sniffed rather than assumed, since it is stored and served verbatim.
			if img.ContentType != tc.wantType {
				t.Errorf("content type = %q, want %q", img.ContentType, tc.wantType)
			}
		})
	}
}

func TestSendsTheKey(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, envelope(jpegBytes))
	}))
	defer server.Close()

	if _, err := New(server.URL, "acct-1", "m", "tok").GenerateImage(context.Background(), "x"); err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if got != "Bearer tok" {
		t.Errorf("Authorization = %q, want Bearer tok", got)
	}
}

func TestReportsProviderFailures(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		contentType string
		body        string
		want        string
	}{
		{
			// Cloudflare returns 200 with success=false often enough that
			// trusting the status line would swallow the failure entirely.
			name:        "a failure inside a 200 is still a failure",
			status:      http.StatusOK,
			contentType: "application/json",
			body:        `{"success":false,"errors":[{"code":10000,"message":"Authentication error"}]}`,
			want:        "Authentication error",
		},
		{
			// This is the class of failure that cost us the FLUX models.
			name:        "a safety refusal is surfaced as written",
			status:      http.StatusBadRequest,
			contentType: "application/json",
			body:        `{"success":false,"errors":[{"code":3030,"message":"Input prompt contains NSFW content."}]}`,
			want:        "NSFW",
		},
		{
			name:        "a bare status is still reported",
			status:      http.StatusServiceUnavailable,
			contentType: "application/json",
			body:        `nope`,
			want:        "status 503",
		},
		{
			// An ingress error arrives as HTML, which the JSON path would never
			// see; without the status check it would look like an empty image.
			name:        "a non-JSON error is still reported",
			status:      http.StatusBadGateway,
			contentType: "text/html",
			body:        `<html>bad gateway</html>`,
			want:        "status 502",
		},
		{
			name:        "success=false with no explanation",
			status:      http.StatusOK,
			contentType: "application/json",
			body:        `{"success":false,"errors":[]}`,
			want:        "failure without an error",
		},
		{
			// Storing undecodable bytes would produce a cover that 200s and
			// renders nothing, which is harder to diagnose than a failure.
			name:        "malformed base64 fails rather than storing garbage",
			status:      http.StatusOK,
			contentType: "application/json",
			body:        `{"success":true,"result":{"image":"!!!not base64!!!"}}`,
			want:        "decoding the image",
		},
		{
			name:        "an empty image is not a result",
			status:      http.StatusOK,
			contentType: "application/json",
			body:        `{"success":true,"result":{"image":""}}`,
			want:        "no image",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, _, _ := serve(t, tc.status, tc.contentType, tc.body)
			_, err := New(server.URL, "acct-1", "m", "tok").GenerateImage(context.Background(), "x")
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
	server, path, _ := serve(t, http.StatusOK, "application/json", envelope(jpegBytes))

	img, err := New(server.URL, "acct-1", "m", "").GenerateImage(context.Background(), "a violet field")
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

// The endpoint is assembled from the account id rather than configured whole,
// so this is the only place a mistake in Cloudflare's URL layout can hide.
func TestDefaultBaseURLBuildsTheRealEndpoint(t *testing.T) {
	got := New("", "acct-1", "@cf/leonardoai/lucid-origin", "tok").endpoint()
	want := "https://api.cloudflare.com/client/v4/accounts/acct-1/ai/run/@cf/leonardoai/lucid-origin"
	if got != want {
		t.Errorf("endpoint = %q, want %q", got, want)
	}
}

// Either credential missing means the provider cannot be called, so both must
// select the placeholder rather than producing a request that 404s.
func TestMissingAccountAlsoRendersThePlaceholder(t *testing.T) {
	server, path, _ := serve(t, http.StatusOK, "application/json", envelope(jpegBytes))

	img, err := New(server.URL, "", "m", "tok").GenerateImage(context.Background(), "x")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if *path != "" {
		t.Errorf("the provider was called at %q despite having no account id", *path)
	}
	if img.ContentType != "image/png" {
		t.Errorf("content type = %q, want the placeholder's image/png", img.ContentType)
	}
}
