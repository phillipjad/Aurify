package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// The cutoff is the whole contract of this thing, so it is what the fake
// records: too old and live runs get reclaimed out from under themselves, too
// new and abandoned ones keep their playlist claimed.
type recordingFailer struct {
	mu      sync.Mutex
	cutoffs []time.Time
	calls   chan struct{}
	err     error
	// failures counts down: while positive, FailStuck errors. Lets a test drive
	// the database going away and coming back.
	failures int
	rows     int64
}

func (f *recordingFailer) FailStuck(_ context.Context, cutoff time.Time) (int64, error) {
	f.mu.Lock()
	f.cutoffs = append(f.cutoffs, cutoff)
	err := f.err
	if f.failures > 0 {
		f.failures--
	} else {
		err = nil
	}
	rows := f.rows
	f.mu.Unlock()

	if f.calls != nil {
		f.calls <- struct{}{}
	}
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func (f *recordingFailer) seen() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time(nil), f.cutoffs...)
}

var testNow = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

// The reap is deliberately unconditional: with one instance, every cover in
// flight when the process starts is dead however recently it was written. A
// cutoff in the past would leave exactly the freshly-orphaned runs claimed,
// which is the case that matters most after a crash.
func TestReapUsesTheCurrentTimeAsItsCutoff(t *testing.T) {
	covers := &recordingFailer{rows: 3}
	sweeper := coverSweeper{
		covers:     covers,
		staleAfter: 2 * time.Minute,
		interval:   time.Hour,
		now:        func() time.Time { return testNow },
	}

	sweeper.Reap(t.Context())

	seen := covers.seen()
	if len(seen) != 1 {
		t.Fatalf("FailStuck called %d times, want 1", len(seen))
	}
	if !seen[0].Equal(testNow) {
		t.Errorf("cutoff = %v, want now (%v); anything earlier spares the runs a crash just orphaned",
			seen[0], testNow)
	}
}

// A database that is unreachable at startup must not take the process with it:
// the covers are still claimed, but the app is otherwise fine and the sweep will
// pick them up.
func TestReapSurvivesAFailingDatabase(t *testing.T) {
	covers := &recordingFailer{err: errors.New("connection refused"), failures: 1}
	sweeper := coverSweeper{covers: covers, staleAfter: time.Minute, interval: time.Hour}

	sweeper.Reap(t.Context()) // must not panic

	if len(covers.seen()) != 1 {
		t.Fatal("the reap did not reach the database")
	}
}

// The sweep looks back by exactly the staleness threshold. Every phase of a live
// run heartbeats, so this is what separates "slow" from "gone".
func TestSweepLooksBackByTheStalenessThreshold(t *testing.T) {
	covers := &recordingFailer{calls: make(chan struct{})}
	sweeper := coverSweeper{
		covers:     covers,
		staleAfter: 2 * time.Minute,
		// Milliseconds, not the production 30s: the interval is a field so a
		// test can drive several ticks without waiting out any of them.
		interval: time.Millisecond,
		now:      func() time.Time { return testNow },
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go sweeper.Sweep(ctx)

	// Three ticks, each unblocked by reading the call it makes.
	for range 3 {
		select {
		case <-covers.calls:
		case <-time.After(2 * time.Second):
			t.Fatal("the sweep stopped ticking")
		}
	}
	cancel()

	want := testNow.Add(-2 * time.Minute)
	for i, cutoff := range covers.seen() {
		if !cutoff.Equal(want) {
			t.Errorf("tick %d cutoff = %v, want %v", i, cutoff, want)
		}
	}
}

// A sweep that gave up on the first error would strand every interrupted run
// from then on, and the failure it is most likely to hit is a transient one.
func TestSweepKeepsGoingAfterAFailedPass(t *testing.T) {
	covers := &recordingFailer{
		calls:    make(chan struct{}),
		err:      errors.New("connection refused"),
		failures: 2,
	}
	sweeper := coverSweeper{
		covers:     covers,
		staleAfter: time.Minute,
		interval:   time.Millisecond,
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go sweeper.Sweep(ctx)

	// Two failing passes, then one that has to still happen.
	for i := range 3 {
		select {
		case <-covers.calls:
		case <-time.After(2 * time.Second):
			t.Fatalf("the sweep stopped after %d passes, the first two of which failed", i)
		}
	}
}

// It is started as a goroutine for the process's lifetime, so shutdown has to
// actually stop it.
func TestSweepStopsWhenTheContextIsDone(t *testing.T) {
	covers := &recordingFailer{}
	sweeper := coverSweeper{covers: covers, staleAfter: time.Minute, interval: time.Millisecond}

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		sweeper.Sweep(ctx)
		close(stopped)
	}()

	cancel()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the sweep outlived its context")
	}
}
