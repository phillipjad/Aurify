// Package imagegen renders a prompt into cover art using Cloudflare Workers AI.
//
// Workers AI was chosen for its free allowance rather than its API: 10,000
// neurons a day, against 57.6 for one FLUX.1 [schnell] image at four steps, is
// roughly 170 images a day at no cost. Its request shape is Cloudflare's own
// rather than OpenAI's, so unlike promptgen this adapter is provider-specific;
// another provider means a sibling adapter behind ports.ImageGenerator (see
// docs/adr/0016-generation-providers.md).
//
// With no account configured the client renders a local placeholder, so a
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

// steps is the number of diffusion steps requested. FLUX.1 [schnell] is
// distilled for very few steps and the model caps this at 8; at 4 the image is
// good and costs 57.6 neurons rather than 96.
const steps = 4

// Client is a Workers AI client.
type Client struct {
	baseURL   string
	accountID string
	model     string
	apiToken  string
	client    *http.Client
}

var _ ports.ImageGenerator = (*Client)(nil)

// New constructs an image-generation client. An empty accountID or apiToken
// enables the placeholder behavior, since neither is usable without the other.
func New(baseURL, accountID, model, apiToken string) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		accountID: accountID,
		model:     model,
		apiToken:  apiToken,
		client:    &http.Client{Timeout: 120 * time.Second},
	}
}

type generateRequest struct {
	Prompt string `json:"prompt"`
	Steps  int    `json:"steps"`
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
	if c.accountID == "" || c.apiToken == "" {
		return placeholderImage(prompt), nil
	}

	body, err := json.Marshal(generateRequest{Prompt: prompt, Steps: steps})
	if err != nil {
		return domain.GeneratedImage{}, err
	}

	endpoint := fmt.Sprintf("%s/accounts/%s/ai/run/%s", c.baseURL, c.accountID, c.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.GeneratedImage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return domain.GeneratedImage{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Decoded before the status is checked: Cloudflare explains a rejection in
	// the errors array, and "status 400" on its own does not say whether the
	// token, the account or the prompt was the problem.
	var out generateResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&out)
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

	raw, err := base64.StdEncoding.DecodeString(out.Result.Image)
	if err != nil {
		return domain.GeneratedImage{}, fmt.Errorf("imagegen: decoding the image: %w", err)
	}
	if len(raw) == 0 {
		return domain.GeneratedImage{}, fmt.Errorf("imagegen: provider returned no image")
	}

	return domain.GeneratedImage{Bytes: raw, ContentType: "image/jpeg"}, nil
}
