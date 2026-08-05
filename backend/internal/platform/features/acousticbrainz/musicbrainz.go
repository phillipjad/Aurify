package acousticbrainz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

// minScore is how good a MusicBrainz match has to be to use.
//
// Their search is fuzzy and always answers with something: a query for a car
// repair video comes back with a confident-looking recording that is not it.
// Exact title and artist matches score 100, so this only admits near-exact hits
// and lets everything else miss, which the cache then remembers as a negative.
const minScore = 90

// mbInterval is MusicBrainz's published rate limit for anonymous callers: one
// request per second, averaged. Exceeding it gets an IP blocked rather than
// throttled, so this is enforced rather than hoped for.
const mbInterval = time.Second

type recordingSearch struct {
	Recordings []struct {
		ID    string `json:"id"`
		Score int    `json:"score"`
	} `json:"recordings"`
}

// resolve returns the MusicBrainz recording ids that plausibly match a track.
//
// More than one is useful rather than wasteful: AcousticBrainz holds data per
// recording, and a song has many recording ids (releases, remasters,
// compilations). Only some carry a submission, so the ids are all handed to the
// batched lookup and whichever answers first wins.
func (c *Client) resolve(ctx context.Context, track domain.Track) ([]string, error) {
	artist := ""
	if len(track.Artists) > 0 {
		artist = track.Artists[0]
	}
	if artist == "" || track.Title == "" {
		return nil, nil
	}

	query := fmt.Sprintf(`artist:%q AND recording:%q`, artist, track.Title)
	endpoint := fmt.Sprintf(
		"%s/ws/2/recording?query=%s&fmt=json&limit=%d",
		c.musicBrainzURL, url.QueryEscape(query), idsPerTrack,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// MusicBrainz requires a descriptive User-Agent and refuses generic ones.
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musicbrainz: status %d", resp.StatusCode)
	}

	var out recordingSearch
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(out.Recordings))
	for _, r := range out.Recordings {
		if r.Score >= minScore && r.ID != "" {
			ids = append(ids, r.ID)
		}
	}
	return ids, nil
}

// limiter serializes outbound calls to one per interval.
//
// A channel rather than a ticker so it holds no goroutine when nothing is
// looking anything up, which is the common case once the cache is warm.
type limiter struct {
	interval time.Duration
	last     time.Time
	mu       chan struct{}
}

func newLimiter(interval time.Duration) *limiter {
	return &limiter{interval: interval, mu: make(chan struct{}, 1)}
}

// wait blocks until the caller may make its request, or the context ends.
func (l *limiter) wait(ctx context.Context) error {
	select {
	case l.mu <- struct{}{}:
		defer func() { <-l.mu }()
	case <-ctx.Done():
		return ctx.Err()
	}

	if delay := l.interval - time.Since(l.last); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	l.last = time.Now()
	return nil
}

// joinIDs builds the semicolon-separated list the AcousticBrainz batch endpoint
// expects.
func joinIDs(ids []string) string { return strings.Join(ids, ";") }
