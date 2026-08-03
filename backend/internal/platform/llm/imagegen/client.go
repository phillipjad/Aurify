// Package imagegen renders a prompt into cover art over the OpenAI images API.
//
// As with promptgen, the API shape is why there is one adapter rather than one
// per vendor: Gemini, OpenAI and Together all serve
// `POST {baseURL}/images/generations`, so changing provider is a change of base
// URL, model and key (see docs/adr/0016-generation-providers.md).
//
// The default is Gemini through its OpenAI-compatibility layer, whose free tier
// is 500 images a day with no card. It replaced Cloudflare Workers AI, whose
// safety classifier refused about a quarter of perfectly ordinary abstract-art
// prompts with no way to opt out.
//
// With no API key configured the client renders a local placeholder, so a
// checkout with no credentials still completes a generation.
package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

const (
	// size keeps covers square. Providers that do not take it ignore it.
	size = "1024x1024"

	// steps is meaningless to Gemini, which silently ignores unknown parameters,
	// and required by the diffusion models behind Together and fal: FLUX.1
	// [schnell] is distilled for very few steps and rejects more than 4, while
	// those APIs default to 20. Sent so pointing at one of them needs no code
	// change.
	steps = 4
)

// Client is an OpenAI-compatible image-generation client.
type Client struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

var _ ports.ImageGenerator = (*Client)(nil)

// New constructs an image-generation client. An empty apiKey enables the
// placeholder behavior: no image API is usable without one, so it is the single
// switch between "configured" and "not".
func New(baseURL, model, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

type generateRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	Size           string `json:"size"`
	Steps          int    `json:"steps"`
	N              int    `json:"n"`
	ResponseFormat string `json:"response_format"`
}

type generateResponse struct {
	Data []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
	// Error is the OpenAI-shaped error body, which carries a far more useful
	// message than the status code alone.
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// GenerateImage renders the prompt and returns the image bytes.
func (c *Client) GenerateImage(ctx context.Context, prompt string) (domain.GeneratedImage, error) {
	if c.apiKey == "" {
		return placeholderImage(prompt), nil
	}

	// Inline bytes rather than a URL: providers hand back links that expire
	// within the hour, and the bytes have to reach ports.ImageStore either way.
	//
	// "b64_json" is OpenAI's own spelling, which Gemini follows. Together spells
	// the same thing "base64", the one place these APIs disagree.
	body, err := json.Marshal(generateRequest{
		Model:          c.model,
		Prompt:         prompt,
		Size:           size,
		Steps:          steps,
		N:              1,
		ResponseFormat: "b64_json",
	})
	if err != nil {
		return domain.GeneratedImage{}, err
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/images/generations", bytes.NewReader(body),
	)
	if err != nil {
		return domain.GeneratedImage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return domain.GeneratedImage{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Decoded before the status is checked, because the body is where a provider
	// explains a rejection.
	var out generateResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&out)
	if out.Error != nil && out.Error.Message != "" {
		return domain.GeneratedImage{}, fmt.Errorf("imagegen: %s", out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return domain.GeneratedImage{}, fmt.Errorf("imagegen: provider returned status %d", resp.StatusCode)
	}
	if decodeErr != nil {
		return domain.GeneratedImage{}, decodeErr
	}
	if len(out.Data) == 0 {
		return domain.GeneratedImage{}, fmt.Errorf("imagegen: provider returned no image")
	}

	raw, err := base64.StdEncoding.DecodeString(out.Data[0].B64JSON)
	if err != nil {
		return domain.GeneratedImage{}, fmt.Errorf("imagegen: decoding the image: %w", err)
	}
	if len(raw) == 0 {
		return domain.GeneratedImage{}, fmt.Errorf("imagegen: provider returned no image")
	}

	// Sniffed rather than assumed: the content type is stored and later served
	// verbatim, and providers differ on whether FLUX comes back as JPEG or PNG.
	return domain.GeneratedImage{Bytes: raw, ContentType: http.DetectContentType(raw)}, nil
}
