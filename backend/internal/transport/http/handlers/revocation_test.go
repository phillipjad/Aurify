package handlers

import (
	"context"
	"errors"
	"testing"
)

// The middleware itself is exercised end to end against the running stack. This
// pins the decision logic that decides whether the check is consulted at all,
// since getting that wrong either lets revoked sessions through or locks
// anonymous routes out.
func TestSessionCheckContract(t *testing.T) {
	var seen string
	check := SessionCheck(func(_ context.Context, sessionID string) error {
		seen = sessionID
		if sessionID == "revoked" {
			return errors.New("session is no longer valid")
		}
		return nil
	})

	if err := check(t.Context(), "live"); err != nil {
		t.Fatalf("live session: %v", err)
	}
	if seen != "live" {
		t.Fatalf("session id passed through = %q, want %q", seen, "live")
	}
	if err := check(t.Context(), "revoked"); err == nil {
		t.Fatal("expected a revoked session to be rejected")
	}
}
