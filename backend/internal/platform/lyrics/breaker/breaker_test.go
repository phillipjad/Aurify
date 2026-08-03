package breaker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/phillipjad/aurify/backend/internal/domain"
)

type stubClient struct {
	calls int
	err   error
	text  string
}

func (s *stubClient) Fetch(context.Context, domain.Track) (string, error) {
	s.calls++
	if s.err != nil {
		return "", s.err
	}
	return s.text, nil
}

// newTestClient wraps a stub with a clock the test drives, so nothing sleeps.
func newTestClient(inner *stubClient, clock *time.Time) *Client {
	c := New(inner)
	c.now = func() time.Time { return *clock }
	return c
}

func fetchN(c *Client, n int) {
	for range n {
		_, _ = c.Fetch(context.Background(), domain.Track{Title: "t"})
	}
}

func TestOpensAfterConsecutiveFailuresAndStopsCalling(t *testing.T) {
	now := time.Now()
	inner := &stubClient{err: errors.New("upstream is unwell")}
	c := newTestClient(inner, &now)

	fetchN(c, failureThreshold)
	if inner.calls != failureThreshold {
		t.Fatalf("inner calls = %d, want %d before the circuit opens", inner.calls, failureThreshold)
	}

	// The point of the exercise: further calls cost the provider nothing.
	fetchN(c, 50)
	if inner.calls != failureThreshold {
		t.Fatalf("inner calls = %d, want the circuit to have stopped them at %d", inner.calls, failureThreshold)
	}
}

// An open circuit must be distinguishable from "this track has no lyrics".
// Returning an empty success instead let the caller cache a skipped track as a
// confirmed absence, so an outage was written into the cache as fact.
func TestOpenCircuitReportsThatItSkipped(t *testing.T) {
	now := time.Now()
	c := newTestClient(&stubClient{err: errors.New("boom")}, &now)
	fetchN(c, failureThreshold)

	text, err := c.Fetch(context.Background(), domain.Track{Title: "t"})
	if !errors.Is(err, ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen so the caller can tell a skip from an absence", err)
	}
	if text != "" {
		t.Fatalf("text = %q, want empty while open", text)
	}
}

func TestProbeAfterCoolOffClosesTheCircuitOnSuccess(t *testing.T) {
	now := time.Now()
	inner := &stubClient{err: errors.New("boom")}
	c := newTestClient(inner, &now)
	fetchN(c, failureThreshold)

	// Still open just before the cool-off elapses.
	now = now.Add(baseCoolOff - time.Second)
	fetchN(c, 1)
	if inner.calls != failureThreshold {
		t.Fatal("a call got through before the cool-off elapsed")
	}

	// Provider recovers; the probe should reach it and close the circuit.
	now = now.Add(2 * time.Second)
	inner.err = nil
	inner.text = "lyrics"
	text, err := c.Fetch(context.Background(), domain.Track{Title: "t"})
	if err != nil || text != "lyrics" {
		t.Fatalf("probe returned (%q, %v), want the provider's answer", text, err)
	}

	fetchN(c, 3)
	if inner.calls != failureThreshold+4 {
		t.Fatalf("inner calls = %d, want the circuit closed and passing everything through", inner.calls)
	}
}

func TestCoolOffGrowsWhileTheProviderStaysDown(t *testing.T) {
	now := time.Now()
	inner := &stubClient{err: errors.New("boom")}
	c := newTestClient(inner, &now)

	fetchN(c, failureThreshold)
	first := c.coolOff

	// Elapse, probe, fail again.
	now = now.Add(first + time.Second)
	fetchN(c, 1)
	second := c.coolOff

	if second <= first {
		t.Fatalf("cool-off did not grow: %s then %s", first, second)
	}
	if first != baseCoolOff {
		t.Fatalf("first cool-off = %s, want %s", first, baseCoolOff)
	}
}

func TestCoolOffIsCapped(t *testing.T) {
	now := time.Now()
	inner := &stubClient{err: errors.New("boom")}
	c := newTestClient(inner, &now)

	fetchN(c, failureThreshold)
	for range 20 {
		now = now.Add(c.coolOff + time.Second)
		fetchN(c, 1)
	}

	if c.coolOff > maxCoolOff {
		t.Fatalf("cool-off = %s, want it capped at %s", c.coolOff, maxCoolOff)
	}
}

// An explicit instruction from the provider beats our guess, but only upwards:
// being told to come back sooner is not a reason to press harder on something
// already failing.
func TestRetryAfterIsHonouredWhenLonger(t *testing.T) {
	now := time.Now()
	inner := &stubClient{err: &RetryAfter{After: 5 * time.Minute, Err: errors.New("too many requests")}}
	c := newTestClient(inner, &now)

	fetchN(c, failureThreshold)

	if c.coolOff != 5*time.Minute {
		t.Fatalf("cool-off = %s, want the provider's 5m instruction", c.coolOff)
	}
}

func TestRetryAfterIsIgnoredWhenShorterThanOurOwnCoolOff(t *testing.T) {
	now := time.Now()
	inner := &stubClient{err: &RetryAfter{After: time.Second, Err: errors.New("too many requests")}}
	c := newTestClient(inner, &now)

	fetchN(c, failureThreshold)

	if c.coolOff != baseCoolOff {
		t.Fatalf("cool-off = %s, want to keep our own %s rather than shorten it", c.coolOff, baseCoolOff)
	}
}

// An intermittent failure must not creep the circuit open: the threshold counts
// consecutive faults, not faults in total.
func TestSuccessResetsTheFailureCount(t *testing.T) {
	now := time.Now()
	inner := &stubClient{}
	c := newTestClient(inner, &now)

	for range 20 {
		inner.err = errors.New("blip")
		fetchN(c, failureThreshold-1)
		inner.err = nil
		fetchN(c, 1)
	}

	if !c.openUntil.IsZero() {
		t.Fatal("the circuit opened despite every run of failures being interrupted by a success")
	}
}
