// Package promptgen talks to the local prompt-generation LLM sidecar, which
// turns a PlaylistAnalysis into an image-generation prompt.
//
// SCAFFOLD: the sidecar service itself is intentionally not built yet (see
// docs/adr/0006-llm-sidecars.md). When AURIFY_PROMPTGEN_URL is unset the client
// returns a deterministic placeholder prompt so the end-to-end pipeline stays
// runnable during development.
package promptgen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Client is an HTTP client for the prompt-generation sidecar.
type Client struct {
	baseURL string
	client  *http.Client
}

var _ ports.PromptGenerator = (*Client)(nil)

// New constructs a prompt-generation client. An empty baseURL enables the
// local placeholder behavior.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

type generateRequest struct {
	Analysis domain.PlaylistAnalysis `json:"analysis"`
}

type generateResponse struct {
	Prompt string `json:"prompt"`
}

// GeneratePrompt returns an image prompt for the analysis.
func (c *Client) GeneratePrompt(ctx context.Context, analysis domain.PlaylistAnalysis) (string, error) {
	if c.baseURL == "" {
		return placeholderPrompt(analysis), nil
	}

	body, err := json.Marshal(generateRequest{Analysis: analysis})
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
		return "", fmt.Errorf("promptgen: sidecar returned status %d", resp.StatusCode)
	}

	var out generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Prompt, nil
}

// placeholderPrompt builds a deterministic prompt from the dominant palette
// dimensions and their colors. It mirrors the shape of a real sidecar's output.
func placeholderPrompt(a domain.PlaylistAnalysis) string {
	var b strings.Builder
	b.WriteString("Abstract album cover: a pleasing amalgamation of geometric shapes, ")
	b.WriteString("composed from a palette of ")

	limit := len(a.Palette)
	if limit > 3 {
		limit = 3
	}
	for i := 0; i < limit; i++ {
		c := a.Palette[i]
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s (%s)", c.Dimension, c.HexColor)
	}

	mood := "balanced"
	switch {
	case a.MeanSentiment.HasLyrics && a.MeanSentiment.Polarity > 0.25:
		mood = "uplifting"
	case a.MeanSentiment.HasLyrics && a.MeanSentiment.Polarity < -0.25:
		mood = "moody"
	}
	fmt.Fprintf(&b, "; %s atmosphere, high-contrast, minimal, no text.", mood)
	return b.String()
}
