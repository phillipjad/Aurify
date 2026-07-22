package handlers

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fgrzl/mux"
)

// Throttle is a fixed-window rate limiter for the unauthenticated auth routes.
//
// It exists for three jobs, all of which are load-bearing rather than hygiene:
//   - Signup reports whether an address is already registered, which makes it an
//     account-existence oracle. Rate limiting is what stops that disclosure
//     scaling into a bulk enumeration of the user table.
//   - Sign-in is the target for credential stuffing and password spraying.
//   - Password reset sends mail, so an open one is a spam relay pointed at
//     whichever addresses an attacker supplies.
//
// ponytail: in-memory and per-process. Behind more than one instance the
// effective limit multiplies by the instance count. Move the counter to Redis or
// a Postgres table if Aurify is ever scaled out, or put the limit at the edge.
type Throttle struct {
	mu      sync.Mutex
	windows map[string]*throttleWindow
	limit   int
	period  time.Duration
	now     func() time.Time
}

type throttleWindow struct {
	count   int
	resetAt time.Time
}

// NewThrottle builds a limiter allowing limit attempts per key per period.
func NewThrottle(limit int, period time.Duration) *Throttle {
	return &Throttle{
		windows: make(map[string]*throttleWindow),
		limit:   limit,
		period:  period,
		now:     time.Now,
	}
}

// Allow records an attempt and reports whether it is within the limit.
func (t *Throttle) Allow(key string) bool {
	now := t.now()

	t.mu.Lock()
	defer t.mu.Unlock()

	// Opportunistic sweep. Without it the map grows once per distinct client
	// forever, which is a slow memory leak an attacker could drive deliberately
	// by rotating source addresses.
	if len(t.windows) > 10_000 {
		for k, w := range t.windows {
			if now.After(w.resetAt) {
				delete(t.windows, k)
			}
		}
	}

	w, ok := t.windows[key]
	if !ok || now.After(w.resetAt) {
		t.windows[key] = &throttleWindow{count: 1, resetAt: now.Add(t.period)}
		return true
	}
	if w.count >= t.limit {
		return false
	}
	w.count++
	return true
}

// allowRequest throttles by client address and, when supplied, by a secondary
// key such as the submitted email.
//
// Limiting on both matters: per-IP alone lets a botnet spray one account from
// many addresses, and per-account alone lets one host walk the whole user table
// one address at a time.
func (t *Throttle) allowRequest(c mux.RouteContext, secondary string) bool {
	ip := clientIP(c.Request())
	if !t.Allow("ip:" + ip) {
		return false
	}
	if secondary != "" {
		return t.Allow("id:" + strings.ToLower(secondary))
	}
	return true
}

// clientIP resolves the caller's address.
//
// X-Forwarded-For is only consulted because mux's forwarded-headers middleware
// is expected to have normalised it upstream. Read straight from the client it
// is trivially spoofed, which would let an attacker mint a fresh rate-limit
// bucket per request and defeat the limiter entirely.
func clientIP(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if first, _, found := strings.Cut(fwd, ","); found {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(fwd)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// tooManyRequests answers a throttled caller.
func tooManyRequests(c mux.RouteContext) {
	c.JSON(http.StatusTooManyRequests, map[string]string{
		"title":  "Too many attempts",
		"detail": "Too many attempts. Wait a few minutes and try again.",
	})
}
