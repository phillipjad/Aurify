// Package imagegen talks to the local image-generation LLM sidecar, which
// renders a prompt into an image and returns its stored URL.
//
// SCAFFOLD: the sidecar service is intentionally not built yet (see
// docs/adr/0006-llm-sidecars.md). When AURIFY_IMAGEGEN_URL is unset the client
// returns a placeholder URL so the pipeline can complete.
package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
)

// Client is an HTTP client for the image-generation sidecar.
type Client struct {
	baseURL string
	client  *http.Client
}

var _ ports.ImageGenerator = (*Client)(nil)

// New constructs an image-generation client. An empty baseURL enables the
// local placeholder behavior.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

type generateRequest struct {
	Prompt string `json:"prompt"`
}

type generateResponse struct {
	ImageURL string `json:"imageUrl"`
}

// GenerateImage renders the prompt and returns the stored image URL.
func (c *Client) GenerateImage(ctx context.Context, prompt string) (string, error) {
	if c.baseURL == "" {
		// Placeholder: a deterministic stand-in URL keyed by the prompt.
		return "https://placehold.co/1024x1024?text=" + url.QueryEscape("aurify"), nil
	}

	body, err := json.Marshal(generateRequest{Prompt: prompt})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("imagegen: sidecar returned status %d", resp.StatusCode)
	}

	var out generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ImageURL, nil
}
