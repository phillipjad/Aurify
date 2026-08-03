// Package promptgen turns a PlaylistAnalysis into an image-generation prompt by
// calling a text model over the OpenAI chat-completions API.
//
// That API shape is the reason there is one adapter rather than one per vendor:
// Ollama serves it on localhost, and so do Groq, OpenRouter, Cerebras and OpenAI
// itself, so moving between a local model and a hosted one is a change of base
// URL rather than of code (see docs/adr/0016-generation-providers.md).
//
// When AURIFY_PROMPTGEN_URL is unset the client returns a deterministic
// placeholder prompt, so a checkout with no model configured still produces
// covers end to end.
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

// Client is an OpenAI-compatible chat-completions client.
type Client struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

var _ ports.PromptGenerator = (*Client)(nil)

// New constructs a prompt-generation client. An empty baseURL enables the
// placeholder behavior. apiKey may be empty: Ollama requires no credential, and
// the header is omitted rather than sent blank so it cannot be mistaken for one.
func New(baseURL, model, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// systemPrompt fixes the output contract. The response is used verbatim as the
// image model's input, so anything conversational around it would be rendered.
const systemPrompt = `You write prompts for an image generation model that ` +
	`produces abstract album cover art.

Given a description of a playlist's mood and color palette, reply with exactly ` +
	`one image prompt. Describe composition, texture, light and color. Use the ` +
	`palette's colors, weighted by how dominant they are.

Reply with the prompt itself and nothing else: no preamble, no explanation, no ` +
	`quotation marks, no markdown. Never ask for text, lettering or words to ` +
	`appear in the image. Keep it under 80 words.`

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	// Error is the OpenAI-shaped error body, which every compatible provider
	// returns with a far more useful message than the status code alone.
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// GeneratePrompt returns an image prompt for the analysis.
func (c *Client) GeneratePrompt(ctx context.Context, analysis domain.PlaylistAnalysis) (string, error) {
	if c.baseURL == "" {
		return placeholderPrompt(analysis), nil
	}

	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: describeAnalysis(analysis)},
		},
		Stream: false,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	// Decoded before the status is checked, because the body is where providers
	// explain a 400.
	var out chatResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&out)
	if out.Error != nil && out.Error.Message != "" {
		return "", fmt.Errorf("promptgen: %s", out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("promptgen: provider returned status %d", resp.StatusCode)
	}
	if decodeErr != nil {
		return "", decodeErr
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("promptgen: provider returned no choices")
	}

	prompt := strings.TrimSpace(stripThinking(out.Choices[0].Message.Content))
	if prompt == "" {
		return "", fmt.Errorf("promptgen: provider returned an empty prompt")
	}
	return prompt, nil
}

// stripThinking removes a leading reasoning block. Reasoning models emit one
// inline whenever the provider does not split it into its own field, and it
// would otherwise be handed to the image model as part of the prompt.
func stripThinking(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "<think>") {
		return s
	}
	if _, after, ok := strings.Cut(trimmed, "</think>"); ok {
		return after
	}
	// An unterminated block means the model spent its whole budget thinking;
	// there is no prompt in there to salvage.
	return ""
}

// describeAnalysis renders the analysis as the plain text the model reasons
// over. Weights are percentages because they read as relative emphasis, which is
// what they are, where three decimal places read as false precision.
func describeAnalysis(a domain.PlaylistAnalysis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "A playlist of %d tracks.\n\nPalette, most dominant first:\n", a.TrackCount)
	for _, c := range a.Palette {
		if c.Weight < 0.01 {
			continue
		}
		fmt.Fprintf(&b, "- %s, %s, %.0f%%\n", c.Dimension, c.HexColor, c.Weight*100)
	}

	if a.MeanSentiment.HasLyrics {
		fmt.Fprintf(&b,
			"\nMean lyric sentiment: polarity %.2f on a scale of -1 (bleak) to 1 (joyful), "+
				"subjectivity %.2f on a scale of 0 (detached) to 1 (personal).\n",
			a.MeanSentiment.Polarity, a.MeanSentiment.Subjectivity,
		)
	} else {
		b.WriteString("\nNo lyrics were available for these tracks.\n")
	}
	return b.String()
}

// placeholderPrompt builds a deterministic prompt from the dominant palette
// dimensions and their colors, for when no model is configured.
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
