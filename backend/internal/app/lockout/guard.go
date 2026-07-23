// Package lockout enforces Aurify's failed-authentication policy: warn after a
// few failures, then permanently lock the (ip, address) pair out.
//
// See docs/adr/0012-account-lockout-policy.md for the rationale and for the
// open question about how a lockout is cleared.
package lockout

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
)

// Policy thresholds.
const (
	// WarnAfter is the failure count at which the user is told they are close
	// to being locked out and pointed at support.
	WarnAfter = 5
	// BlockAfter is the failure count within Window that triggers a permanent
	// lockout of the pair.
	BlockAfter = 10
	// Window is the period the failure count is measured over.
	Window = 15 * time.Minute
)

// Decision is the outcome of consulting the guard.
type Decision struct {
	// Blocked means the pair is permanently locked out. It never becomes false
	// again without operator action.
	Blocked bool
	// Failures is the count within the current window.
	Failures int
	// Warn means the caller should tell the user how close they are and how to
	// reach support.
	Warn bool
}

// Guard tracks failed authentication attempts.
//
// The rolling count lives in memory because it is cheap and disposable: losing
// it on restart costs an attacker at most one extra window. The permanent block
// lives in PostgreSQL, because losing that on restart would make "permanent"
// false in exactly the case it matters.
//
// ponytail: the in-memory counter is per-process, so behind N instances an
// attacker gets up to N times the failures before tripping the block. The
// permanent block is shared, so it still takes effect everywhere once written.
// Move the counter to Redis if Aurify is scaled out.
type Guard struct {
	blocks ports.AuthBlockRepository

	mu      sync.Mutex
	windows map[string]*counter

	warnAfter  int
	blockAfter int
	window     time.Duration
	now        func() time.Time
}

type counter struct {
	failures int
	resetAt  time.Time
}

// NewGuard constructs a Guard using the package thresholds.
func NewGuard(blocks ports.AuthBlockRepository) *Guard {
	return &Guard{
		blocks:     blocks,
		windows:    make(map[string]*counter),
		warnAfter:  WarnAfter,
		blockAfter: BlockAfter,
		window:     Window,
		now:        time.Now,
	}
}

// Check reports the current standing of a pair without recording an attempt.
// Handlers call it before doing any work, so a blocked pair never reaches the
// password hashing or the database.
func (g *Guard) Check(ctx context.Context, ip, identifier string) (Decision, error) {
	identifier = normalize(identifier)

	blocked, err := g.blocks.IsBlocked(ctx, ip, identifier)
	if err != nil {
		return Decision{}, err
	}
	if blocked {
		return Decision{Blocked: true}, nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	failures := g.currentLocked(ip, identifier)
	return Decision{Failures: failures, Warn: failures >= g.warnAfter}, nil
}

// RecordFailure counts one failed attempt and returns the resulting standing.
// Crossing the block threshold writes the permanent lockout before returning.
func (g *Guard) RecordFailure(ctx context.Context, ip, identifier string) (Decision, error) {
	identifier = normalize(identifier)
	now := g.now()

	g.mu.Lock()
	g.sweepLocked(now)
	key := key(ip, identifier)
	c, ok := g.windows[key]
	if !ok || now.After(c.resetAt) {
		c = &counter{resetAt: now.Add(g.window)}
		g.windows[key] = c
	}
	c.failures++
	failures := c.failures
	g.mu.Unlock()

	if failures >= g.blockAfter {
		if err := g.blocks.Block(ctx, ip, identifier, failures, "repeated failed authentication"); err != nil {
			return Decision{}, err
		}
		return Decision{Blocked: true, Failures: failures}, nil
	}
	return Decision{Failures: failures, Warn: failures >= g.warnAfter}, nil
}

// Reset clears the rolling failure count after a success.
//
// It deliberately does not clear a permanent block. Once a pair is locked out,
// signing in correctly is not a way back: if it were, an attacker who
// eventually guessed the password would immediately erase the evidence and the
// lockout.
func (g *Guard) Reset(ip, identifier string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.windows, key(ip, normalize(identifier)))
}

func (g *Guard) currentLocked(ip, identifier string) int {
	c, ok := g.windows[key(ip, identifier)]
	if !ok || g.now().After(c.resetAt) {
		return 0
	}
	return c.failures
}

// sweepLocked drops expired counters. Without it the map grows once per
// distinct pair forever, which an attacker could drive deliberately by rotating
// source addresses.
func (g *Guard) sweepLocked(now time.Time) {
	if len(g.windows) < 10_000 {
		return
	}
	for k, c := range g.windows {
		if now.After(c.resetAt) {
			delete(g.windows, k)
		}
	}
}

func key(ip, identifier string) string { return ip + "\x00" + identifier }

func normalize(identifier string) string {
	return strings.ToLower(strings.TrimSpace(identifier))
}
