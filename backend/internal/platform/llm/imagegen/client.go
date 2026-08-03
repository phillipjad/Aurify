// Package imagegen renders a prompt into cover art using Cloudflare Workers AI.
//
// Workers AI is here for one reason: it is the only image provider found that is
// free without a card, a deposit or an expiry. Together AI now wants a $5
// deposit and Gemini's image models are not free-tier eligible at all.
//
// Unlike promptgen it is provider-shaped rather than OpenAI-shaped, because
// Workers AI serves OpenAI compatibility for text only. Moving to an
// OpenAI-compatible image API later means a sibling adapter behind
// ports.ImageGenerator; see docs/adr/0016-generation-providers.md.
//
// The model deliberately is not FLUX. Cloudflare's FLUX endpoints run a safety
// classifier that refused 2 of 8 measured generations of ordinary abstract-art
// prompts, with no way to opt out.
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
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Square, because album art is square. Both dimensions are sent because Workers
// AI takes them separately rather than as one "size" string.
const (
	imageWidth  = 1024
	imageHeight = 1024
)

// Client is a Workers AI image client.
//
// The account id lives inside baseURL rather than in a setting of its own, so
// this takes the same three values as promptgen: where, which model, and the
// credential.
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
	Prompt string `json:"prompt"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// generateResponse is Cloudflare's envelope, which reports failure in the body
// rather than only in the status line.
type generateResponse struct {
	Result struct {
		Image string `json:"image"`
	} `json:"result"`
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// GenerateImage renders the prompt and returns the image bytes.
func (c *Client) GenerateImage(ctx context.Context, prompt string) (domain.GeneratedImage, error) {
	if c.apiKey == "" {
		return placeholderImage(prompt), nil
	}

	body, err := json.Marshal(generateRequest{
		Prompt: prompt,
		Width:  imageWidth,
		Height: imageHeight,
	})
	if err != nil {
		return domain.GeneratedImage{}, err
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/"+c.model, bytes.NewReader(body),
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

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.GeneratedImage{}, err
	}

	// Workers AI answers in one of two shapes depending on the model: the newer
	// ones wrap base64 in Cloudflare's JSON envelope, the Stable Diffusion ones
	// stream the image itself. Switching on the content type rather than the
	// model name means trying a different model is a config change.
	if !strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		if resp.StatusCode != http.StatusOK {
			return domain.GeneratedImage{}, fmt.Errorf(
				"imagegen: provider returned status %d", resp.StatusCode,
			)
		}
		return decoded(raw)
	}

	var out generateResponse
	decodeErr := json.Unmarshal(raw, &out)
	// Checked before the status, because Cloudflare explains a rejection here and
	// "status 400" alone does not say whether the token, the model or the prompt
	// was the problem.
	if len(out.Errors) > 0 {
		return domain.GeneratedImage{}, fmt.Errorf("imagegen: %s", out.Errors[0].Message)
	}
	if resp.StatusCode != http.StatusOK {
		return domain.GeneratedImage{}, fmt.Errorf("imagegen: provider returned status %d", resp.StatusCode)
	}
	if decodeErr != nil {
		return domain.GeneratedImage{}, decodeErr
	}
	if !out.Success {
		return domain.GeneratedImage{}, fmt.Errorf("imagegen: provider reported failure without an error")
	}

	image, err := base64.StdEncoding.DecodeString(out.Result.Image)
	if err != nil {
		return domain.GeneratedImage{}, fmt.Errorf("imagegen: decoding the image: %w", err)
	}
	return decoded(image)
}

// decoded wraps raw image bytes, sniffing the content type rather than assuming
// it: the type is stored and later served verbatim, and Workers AI models differ
// on JPEG versus PNG.
func decoded(raw []byte) (domain.GeneratedImage, error) {
	if len(raw) == 0 {
		return domain.GeneratedImage{}, fmt.Errorf("imagegen: provider returned no image")
	}
	return domain.GeneratedImage{Bytes: raw, ContentType: http.DetectContentType(raw)}, nil
}
