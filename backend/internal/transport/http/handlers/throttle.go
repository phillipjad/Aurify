package handlers

import (
	"net"
	"net/http"
	"strings"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app/lockout"
)

// clientIP resolves the caller's address.
//
// X-Forwarded-For is only consulted because mux's forwarded-headers middleware
// is expected to have normalised it upstream. Read straight from an untrusted
// client it is trivially spoofed, which would let an attacker mint a fresh
// lockout bucket per request and defeat the policy entirely.
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

// respondBlocked answers a permanently locked-out caller.
//
// It is a 403 rather than a 429: 429 invites the client to retry later, and
// there is no later. The support address is the only route back.
func respondBlocked(c mux.RouteContext, supportEmail string) {
	c.JSON(http.StatusForbidden, map[string]string{
		"title": "Access blocked",
		"detail": "Too many failed attempts from this location for this account. " +
			"This block is permanent and must be lifted by us. Contact " + supportEmail + " to restore access.",
	})
}

// warning is appended to a failure response once the user is close to being
// locked out, so the block is never a surprise.
func warning(decision lockout.Decision, supportEmail string) string {
	if !decision.Warn {
		return ""
	}
	remaining := lockout.BlockAfter - decision.Failures
	if remaining < 1 {
		remaining = 1
	}
	return " After " + itoa(remaining) + " more failed attempt(s) this account will be blocked from this location " +
		"and only we can unblock it. If you are stuck, contact " + supportEmail + "."
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
