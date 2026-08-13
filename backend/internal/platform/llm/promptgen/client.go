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
//
// It deliberately does not name a house style. An earlier version asked for
// "composition, texture, light and color", which reliably produced painterly
// impasto for every playlist: the wording, not the music, was choosing the look.
// What the cover looks like comes from the visual language in the user message,
// which is derived from the palette.
const systemPrompt = `You write prompts for an image generation model that ` +
	`produces abstract album cover art.

You are given a playlist's color palette and a visual language, both weighted. ` +
	`Reply with exactly one image prompt that blends them in roughly those ` +
	`proportions: a dimension at 50% should dominate the image, one at 10% should ` +
	`be a trace. Do not pick a single style and ignore the rest, and do not fall ` +
	`back on a default look of your own.

Reply with the prompt itself and nothing else: no preamble, no explanation, no ` +
	`quotation marks, no markdown. Never ask for text, lettering or words to ` +
	`appear in the image. Keep it under 80 words.`

// visualLanguage maps a palette dimension to the look it contributes.
//
// The keys are the dimension names in internal/analysis/weights.go. Style is
// driven by the same weights as color, so the two cannot disagree, and a
// playlist that is 60% melancholic gets a cover that is 60% that atmosphere
// rather than a painterly one with purple in it.
//
// ponytail: an unknown dimension is skipped rather than failing. Adding one in
// weights.go without adding it here quietly loses its contribution to the look;
// the palette still carries its color.
var visualLanguage = map[string]string{
	"energetic":     "sharp angular fragments, kinetic diagonals, hard edges",
	"danceable":     "repeating rhythmic geometry, pattern and pulse",
	"euphoric":      "radiant blooming light, soft bursts, high key",
	"organic":       "natural grain, fibre and weathered surfaces",
	"introspective": "sparse minimal geometry, wide negative space, stillness",
	"melancholic":   "soft diffuse washes, heavy atmosphere, low light",
	"intimate":      "close fine detail, delicate line work, small scale",
	"driving":       "insistent forward motion, streaked repetition, momentum",
}

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
	fmt.Fprintf(&b, "A playlist of %d tracks.\n", a.TrackCount)

	// An absent palette has to be said out loud. The system prompt forbids the
	// model falling back on a look of its own, so leaving the section empty
	// asks it to disobey that or to invent the playlist's character silently.
	if len(a.Palette) == 0 {
		b.WriteString("\nNothing measurable is known about how this playlist sounds: " +
			"no track matched an acoustic analysis and no estimate was available. " +
			"Compose something neutral and abstract that commits to no particular mood.\n")
	} else {
		b.WriteString("\nPalette, most dominant first:\n")
		for _, c := range a.Palette {
			if c.Weight < 0.01 {
				continue
			}
			fmt.Fprintf(&b, "- %s, %s, %.0f%%\n", c.Dimension, c.HexColor, c.Weight*100)
		}

		// The same weights again, as look rather than color. Emitted as its own
		// section so the model is asked to blend two aligned things rather than
		// to infer a style from hex codes.
		b.WriteString("\nVisual language, blend in these proportions:\n")
		for _, c := range a.Palette {
			if c.Weight < 0.01 {
				continue
			}
			if language, ok := visualLanguage[c.Dimension]; ok {
				fmt.Fprintf(&b, "- %.0f%% %s\n", c.Weight*100, language)
			}
		}
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
	b.WriteString("Abstract album cover: a pleasing amalgamation of geometric shapes")

	// Guarded: a playlist with no palette would otherwise be composed "from a
	// palette of ; balanced atmosphere".
	if len(a.Palette) > 0 {
		b.WriteString(", composed from a palette of ")
		limit := min(len(a.Palette), 3)
		for i := 0; i < limit; i++ {
			c := a.Palette[i]
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s (%s)", c.Dimension, c.HexColor)
		}
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
