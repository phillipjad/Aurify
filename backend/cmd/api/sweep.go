package main

import (
	"context"
	"log/slog"
	"time"
)

// stuckCoverFailer is the one operation the sweeper needs. Declared here rather
// than on ports because failing abandoned covers is operational recovery, not an
// application command (see the FailStuck comment in internal/storage/postgres).
type stuckCoverFailer interface {
	FailStuck(ctx context.Context, cutoff time.Time) (int64, error)
}

// coverSweeper releases the claim a generation holds on its playlist when the
// generation is no longer running (ADR 0022).
//
// Generation runs in-process (ADR 0019), so a crash orphans in-flight covers in
// a non-terminal status, and a cover in that state refuses new generations of
// its playlist. Something has to notice. Two things do, and they answer
// different failures — see Reap and Sweep.
//
// The durations and the clock are fields rather than constants so a test can
// drive both without waiting out a real interval.
type coverSweeper struct {
	covers stuckCoverFailer
	// staleAfter is how long a cover may go untouched before Sweep treats it as
	// abandoned. It has to clear the longest gap a *live* run leaves between
	// writes, which is the pipeline's heartbeat interval.
	staleAfter time.Duration
	// interval is how often Sweep looks.
	interval time.Duration
	// now is the clock, injectable for tests. Nil means time.Now.
	now func() time.Time
}

func (s coverSweeper) clock() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now()
}

// Reap fails every cover currently in flight, with no staleness threshold at
// all, and is meant to be called once at startup before the server accepts
// anything.
//
// It can be that blunt because the service is pinned to a single instance (see
// deploy/app/main.tf): a claim is only ever held by a goroutine in this process,
// so nothing that was generating before this process started is generating now,
// whatever its timestamp says. Waiting out a threshold would only leave those
// playlists unusable for longer.
//
// During a deployment the outgoing revision may still be finishing a run when
// this one starts, and this reaps it. That run was already doomed — Cloud Run is
// draining that instance and shutdown does not wait — and if it does happen to
// finish, its terminal write simply lands after ours.
func (s coverSweeper) Reap(ctx context.Context) {
	n, err := s.covers.FailStuck(ctx, s.clock())
	if err != nil {
		slog.Warn("reaping interrupted covers failed", "error", err)
		return
	}
	if n > 0 {
		slog.Info("reaped covers interrupted by a restart", "count", n)
	}
}

// Sweep runs until ctx is done, failing covers that have gone quiet.
//
// This is the backstop for a run lost without the process dying, which Reap
// cannot see because there is no restart. A failure to reach the database is
// logged and retried on the next tick rather than ending the loop: the sweeper
// giving up permanently would strand every future interrupted run.
func (s coverSweeper) Sweep(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		n, err := s.covers.FailStuck(ctx, s.clock().Add(-s.staleAfter))
		if err != nil {
			slog.Warn("sweeping stuck covers failed", "error", err)
			continue
		}
		if n > 0 {
			slog.Info("failed stuck covers", "count", n)
		}
	}
}
