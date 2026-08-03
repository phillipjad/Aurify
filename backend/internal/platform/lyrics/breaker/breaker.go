// Package breaker wraps a lyrics client so a failing provider is left alone
// instead of being hammered.
//
// The policy is to stop calling, not to retry. Retrying adds load exactly when
// the service can least take it, and a lyric is a best-effort signal the
// pipeline already treats as optional, so skipping is a legitimate answer where
// for most resources it would not be.
package breaker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

const (
	// failureThreshold is how many consecutive faults open the circuit.
	failureThreshold = 5
	// baseCoolOff is the first pause, doubling on each subsequent failure.
	baseCoolOff = 30 * time.Second
	// maxCoolOff caps the growth, so a provider that comes back is noticed
	// within a few minutes rather than hours.
	maxCoolOff = 10 * time.Minute
)

// ErrOpen reports that the circuit is open and no call was attempted.
//
// It is an error rather than an empty success on purpose. "We did not ask" and
// "we asked and there are no lyrics" look identical to a caller otherwise, and
// conflating them let an outage be written into the cache as fact: a skipped
// track was stored as found=false and stayed that way for the negative TTL,
// so one bad spell at the provider marked a whole library as lyric-less.
var ErrOpen = errors.New("breaker: lyrics provider circuit is open")

// RetryAfter is returned by a client that was told when to come back. The
// breaker honours it over its own schedule when it is longer.
type RetryAfter struct {
	After time.Duration
	Err   error
}

func (e *RetryAfter) Error() string { return e.Err.Error() }
func (e *RetryAfter) Unwrap() error { return e.Err }

// Client wraps a ports.LyricsClient with a circuit breaker.
type Client struct {
	inner ports.LyricsClient
	now   func() time.Time

	mu        sync.Mutex
	failures  int
	openUntil time.Time
	coolOff   time.Duration
}

var _ ports.LyricsClient = (*Client)(nil)

// New wraps a client.
func New(inner ports.LyricsClient) *Client {
	return &Client{inner: inner, now: time.Now}
}

// Fetch calls through unless the circuit is open, in which case it fails fast
// with ErrOpen without touching the provider.
func (c *Client) Fetch(ctx context.Context, track domain.Track) (string, error) {
	if !c.allow() {
		return "", ErrOpen
	}

	text, err := c.inner.Fetch(ctx, track)
	if err != nil {
		c.recordFailure(err)
		return "", err
	}
	c.recordSuccess()
	return text, nil
}

// allow reports whether a call may proceed, letting a single probe through once
// the cool-off has elapsed.
func (c *Client) allow() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.openUntil.IsZero() || c.now().After(c.openUntil) {
		return true
	}
	return false
}

func (c *Client) recordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.failures > 0 || !c.openUntil.IsZero() {
		slog.Info("lyrics provider recovered, circuit closed")
	}
	c.failures = 0
	c.coolOff = 0
	c.openUntil = time.Time{}
}

func (c *Client) recordFailure(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.failures++
	if c.failures < failureThreshold {
		return
	}

	c.coolOff = min(max(c.coolOff*2, baseCoolOff), maxCoolOff)

	// An explicit instruction from the provider beats our guess, but only
	// upwards: being told to come back sooner than we intended is not a reason
	// to press harder on something that is already failing.
	var retry *RetryAfter
	if errors.As(err, &retry) && retry.After > c.coolOff {
		c.coolOff = retry.After
	}

	c.openUntil = c.now().Add(c.coolOff)
	slog.Warn("lyrics provider failing, circuit opened",
		"failures", c.failures, "cool_off", c.coolOff, "error", err)
}
