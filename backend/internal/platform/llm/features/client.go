// Package features estimates a playlist's acoustic character from its track
// titles, using the text model already configured for prompt generation.
//
// This is the fallback, not the plan. Real features come from AcousticBrainz;
// this runs only when nothing in a playlist could be matched there, which is
// what happens to anything released after their 2022 freeze. What it produces is
// a guess, and domain.PlaylistAnalysis.FeaturesEstimated records that it was
// used so nothing downstream mistakes it for a measurement.
//
// Measured against gemma3:4b on four playlists it does discriminate:
// instrumentalness spread 0.78 across genres, valence 0.35, energy 0.33. It also
// got acousticness plainly wrong, calling power metal more acoustic than
// country, and returned an identical speechiness for everything.
package features

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// maxTracksInPrompt caps how much of a playlist is described.
//
// The estimate is one number per dimension for the whole playlist, so a sample
// characterizes it as well as the full list would, and a 194-track playlist
// would otherwise spend most of the context window listing songs.
const maxTracksInPrompt = 40

const systemPrompt = `You estimate the acoustic character of a playlist from its track list.

Reply with ONLY a JSON object with these keys, each a number from 0.0 to 1.0: ` +
	`acousticness, danceability, energy, instrumentalness, liveness, loudness, ` +
	`speechiness, valence. Base it on what you know of these artists and songs.

No prose, no explanation, no markdown.`

// jsonObject finds the first {...} block in a reply.
//
// Needed because the model wraps its answer in a ```json fence even when asked
// for a JSON object through response_format, so unmarshalling the whole reply
// fails on every single call.
var jsonObject = regexp.MustCompile(`(?s)\{.*\}`)

// Client estimates features over the OpenAI chat-completions API.
type Client struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

var _ ports.FeatureEstimator = (*Client)(nil)

// New constructs an estimator. An empty baseURL disables estimation, which is
// the same switch promptgen uses and means an unconfigured checkout simply keeps
// today's behaviour.
func New(baseURL, model, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Stream         bool            `json:"stream"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// estimate is the model's reply. Every field is a pointer so an absent key is
// distinguishable from a genuine zero, which matters because "energy: 0" and
// "the model forgot energy" should not mean the same thing.
type estimate struct {
	Acousticness     *float64 `json:"acousticness"`
	Danceability     *float64 `json:"danceability"`
	Energy           *float64 `json:"energy"`
	Instrumentalness *float64 `json:"instrumentalness"`
	Liveness         *float64 `json:"liveness"`
	Loudness         *float64 `json:"loudness"`
	Speechiness      *float64 `json:"speechiness"`
	Valence          *float64 `json:"valence"`
}

// Estimate returns the playlist's estimated character, or Present false.
func (c *Client) Estimate(ctx context.Context, tracks []domain.Track) (domain.AudioFeatures, error) {
	if c.baseURL == "" || len(tracks) == 0 {
		return domain.AudioFeatures{Present: false}, nil
	}

	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: describeTracks(tracks)},
		},
		Stream:         false,
		ResponseFormat: &responseFormat{Type: "json_object"},
	})
	if err != nil {
		return domain.AudioFeatures{}, err
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body),
	)
	if err != nil {
		return domain.AudioFeatures{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return domain.AudioFeatures{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var out chatResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&out)
	if out.Error != nil && out.Error.Message != "" {
		return domain.AudioFeatures{}, fmt.Errorf("features: %s", out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return domain.AudioFeatures{}, fmt.Errorf("features: provider returned status %d", resp.StatusCode)
	}
	if decodeErr != nil {
		return domain.AudioFeatures{}, decodeErr
	}
	if len(out.Choices) == 0 {
		return domain.AudioFeatures{}, fmt.Errorf("features: provider returned no choices")
	}

	return parseEstimate(out.Choices[0].Message.Content)
}

// parseEstimate turns the model's reply into features, clamping as it goes.
func parseEstimate(content string) (domain.AudioFeatures, error) {
	block := jsonObject.FindString(content)
	if block == "" {
		return domain.AudioFeatures{}, fmt.Errorf("features: no JSON object in the reply")
	}

	var e estimate
	if err := json.Unmarshal([]byte(block), &e); err != nil {
		return domain.AudioFeatures{}, fmt.Errorf("features: parsing the estimate: %w", err)
	}

	f := domain.AudioFeatures{
		Acousticness:     value(e.Acousticness),
		Danceability:     value(e.Danceability),
		Energy:           value(e.Energy),
		Instrumentalness: value(e.Instrumentalness),
		Liveness:         value(e.Liveness),
		Loudness:         value(e.Loudness),
		Speechiness:      value(e.Speechiness),
		Valence:          value(e.Valence),
	}

	// A reply that named nothing usable is a failure, not a silent playlist. A
	// zeroed AudioFeatures is exactly the bug this whole change exists to fix.
	if e.Acousticness == nil && e.Danceability == nil && e.Energy == nil && e.Valence == nil {
		return domain.AudioFeatures{}, fmt.Errorf("features: the estimate named no usable dimension")
	}
	f.Present = true
	return f, nil
}

// value clamps into [0,1]. The model returns 1.5 and -0.2 often enough that an
// unclamped value would quietly skew a palette.
func value(v *float64) float64 {
	if v == nil {
		return 0
	}
	switch {
	case *v < 0:
		return 0
	case *v > 1:
		return 1
	default:
		return *v
	}
}

// describeTracks renders the track list the model reasons over.
func describeTracks(tracks []domain.Track) string {
	var b strings.Builder
	b.WriteString("Tracks:\n")

	limit := min(len(tracks), maxTracksInPrompt)
	for _, t := range tracks[:limit] {
		artist := ""
		if len(t.Artists) > 0 {
			artist = t.Artists[0]
		}
		if artist != "" {
			fmt.Fprintf(&b, "- %s - %s\n", artist, t.Title)
			continue
		}
		fmt.Fprintf(&b, "- %s\n", t.Title)
	}
	if len(tracks) > limit {
		fmt.Fprintf(&b, "(and %d more)\n", len(tracks)-limit)
	}
	return b.String()
}
