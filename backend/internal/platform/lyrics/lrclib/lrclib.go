// Package lrclib is a client for lrclib.net, used to fetch plain-text lyrics
// for sentiment analysis. This adapter is implemented (not stubbed): lrclib has
// a simple, key-less public API.
package lrclib

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/lyrics/breaker"
)

// DefaultBaseURL is the public lrclib endpoint.
const DefaultBaseURL = "https://lrclib.net"

// Client fetches lyrics from an lrclib-compatible host.
type Client struct {
	baseURL string
	version string
	client  *http.Client
}

var _ ports.LyricsClient = (*Client)(nil)

// New constructs a client. An empty baseURL falls back to DefaultBaseURL.
func New(baseURL, version string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		version: version,
		// A healthy lookup answers in about a quarter of a second. Ten seconds was
		// generous to the point of being harmful: with a provider timing out, each
		// track cost ten seconds before the circuit breaker upstream had seen
		// enough failures to stop asking.
		client: &http.Client{Timeout: 4 * time.Second},
	}
}

// parseRetryAfter reads the header in both forms RFC 9110 allows: a delay in
// seconds, or an HTTP date. A past date or unparseable value reports false, so
// the caller falls back on its own schedule.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	if when, err := http.ParseTime(value); err == nil {
		if d := when.Sub(now); d > 0 {
			return d, true
		}
	}
	return 0, false
}

type getResponse struct {
	PlainLyrics  string `json:"plainLyrics"`
	SyncedLyrics string `json:"syncedLyrics"`
	Instrumental bool   `json:"instrumental"`
}

// Fetch returns plain-text lyrics for a track. A track with no match returns
// ("", nil) — callers treat missing lyrics as a neutral signal, not an error.
func (c *Client) Fetch(ctx context.Context, track domain.Track) (string, error) {
	q := url.Values{}
	q.Set("track_name", track.Title)
	if len(track.Artists) > 0 {
		q.Set("artist_name", track.Artists[0])
	}
	if track.Album != "" {
		q.Set("album_name", track.Album)
	}
	if track.DurationMS > 0 {
		q.Set("duration", strconv.Itoa(track.DurationMS/1000))
	}

	endpoint := c.baseURL + "/api/get?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	// lrclib asks clients to identify themselves.
	req.Header.Set("User-Agent", "Aurify "+c.version+" (https://github.com/phillipjad/aurify)")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		// handled below
	case http.StatusNotFound:
		// Not a failure: lrclib simply has nothing for this track, which is the
		// common case for a video-centric library.
		return "", nil
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		// lrclib does not send Retry-After today, but if it ever starts, an
		// explicit instruction is worth more than the breaker's guess.
		err := fmt.Errorf("lrclib: unexpected status %d", resp.StatusCode)
		if after, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
			return "", &breaker.RetryAfter{After: after, Err: err}
		}
		return "", err
	default:
		return "", fmt.Errorf("lrclib: unexpected status %d", resp.StatusCode)
	}

	var out getResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Instrumental {
		return "", nil
	}
	if out.PlainLyrics != "" {
		return out.PlainLyrics, nil
	}
	return out.SyncedLyrics, nil
}
