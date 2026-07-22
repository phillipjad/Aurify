package lockout

import (
	"context"
	"testing"
	"time"
)

// fakeBlocks is an in-memory ports.AuthBlockRepository.
type fakeBlocks struct {
	blocked map[string]int // key -> failures recorded at block time
}

func newFakeBlocks() *fakeBlocks { return &fakeBlocks{blocked: map[string]int{}} }

func (f *fakeBlocks) IsBlocked(_ context.Context, ip, identifier string) (bool, error) {
	_, ok := f.blocked[ip+"|"+identifier]
	return ok, nil
}

func (f *fakeBlocks) Block(_ context.Context, ip, identifier string, failures int, _ string) error {
	key := ip + "|" + identifier
	if _, exists := f.blocked[key]; !exists {
		f.blocked[key] = failures
	}
	return nil
}

func TestWarnsAfterFiveFailures(t *testing.T) {
	blocks := newFakeBlocks()
	g := NewGuard(blocks)

	for i := 1; i <= 4; i++ {
		d, err := g.RecordFailure(t.Context(), "1.2.3.4", "user@example.com")
		if err != nil {
			t.Fatalf("failure %d: %v", i, err)
		}
		if d.Warn {
			t.Fatalf("warned early at failure %d", i)
		}
		if d.Blocked {
			t.Fatalf("blocked early at failure %d", i)
		}
	}

	d, err := g.RecordFailure(t.Context(), "1.2.3.4", "user@example.com")
	if err != nil {
		t.Fatalf("fifth failure: %v", err)
	}
	if !d.Warn {
		t.Fatal("expected a warning on the fifth failure")
	}
	if d.Blocked {
		t.Fatal("must not be blocked on the fifth failure")
	}
}

func TestBlocksPermanentlyAtTenFailures(t *testing.T) {
	blocks := newFakeBlocks()
	g := NewGuard(blocks)

	var last Decision
	for i := 1; i <= BlockAfter; i++ {
		d, err := g.RecordFailure(t.Context(), "1.2.3.4", "user@example.com")
		if err != nil {
			t.Fatalf("failure %d: %v", i, err)
		}
		last = d
	}
	if !last.Blocked {
		t.Fatalf("expected a block at %d failures, got %+v", BlockAfter, last)
	}

	// The block must be durable, not just a state in the counter.
	if ok, _ := blocks.IsBlocked(t.Context(), "1.2.3.4", "user@example.com"); !ok {
		t.Fatal("block was not persisted")
	}

	// And it must be observed by a fresh Check, including from a brand new
	// process that has no in-memory counter at all.
	fresh := NewGuard(blocks)
	d, err := fresh.Check(t.Context(), "1.2.3.4", "user@example.com")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !d.Blocked {
		t.Fatal("a restarted process did not see the persisted block")
	}
}

// The block keys on the pair, so locking out one attacker must not lock the
// real user out of their own account from their own address. Keying on the
// address alone would turn this control into a denial-of-service tool.
func TestBlockIsScopedToTheIPAndAddressPair(t *testing.T) {
	blocks := newFakeBlocks()
	g := NewGuard(blocks)

	for i := 0; i < BlockAfter; i++ {
		if _, err := g.RecordFailure(t.Context(), "9.9.9.9", "victim@example.com"); err != nil {
			t.Fatalf("failure %d: %v", i, err)
		}
	}

	// Same account, different source: unaffected.
	d, err := g.Check(t.Context(), "1.2.3.4", "victim@example.com")
	if err != nil {
		t.Fatalf("check other ip: %v", err)
	}
	if d.Blocked {
		t.Fatal("the victim was locked out of their own account from their own address")
	}

	// Same source, different account: also unaffected, so one bad actor behind a
	// shared NAT address does not take out everyone behind it.
	d, err = g.Check(t.Context(), "9.9.9.9", "someone-else@example.com")
	if err != nil {
		t.Fatalf("check other identifier: %v", err)
	}
	if d.Blocked {
		t.Fatal("an unrelated account sharing the source address was locked out")
	}
}

func TestResetClearsTheRollingCountButNotABlock(t *testing.T) {
	blocks := newFakeBlocks()
	g := NewGuard(blocks)

	for i := 0; i < 3; i++ {
		if _, err := g.RecordFailure(t.Context(), "1.2.3.4", "user@example.com"); err != nil {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	g.Reset("1.2.3.4", "user@example.com")

	d, err := g.Check(t.Context(), "1.2.3.4", "user@example.com")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if d.Failures != 0 {
		t.Fatalf("failures = %d after reset, want 0", d.Failures)
	}

	// Now drive it to a permanent block and confirm a success cannot undo it.
	// If signing in correctly cleared a block, an attacker who eventually
	// guessed the password would erase the lockout and the evidence with it.
	for i := 0; i < BlockAfter; i++ {
		if _, err := g.RecordFailure(t.Context(), "1.2.3.4", "user@example.com"); err != nil {
			t.Fatalf("failure %d: %v", i, err)
		}
	}
	g.Reset("1.2.3.4", "user@example.com")

	d, err = g.Check(t.Context(), "1.2.3.4", "user@example.com")
	if err != nil {
		t.Fatalf("check after reset: %v", err)
	}
	if !d.Blocked {
		t.Fatal("Reset cleared a permanent block")
	}
}

func TestFailureCountExpiresWithTheWindow(t *testing.T) {
	blocks := newFakeBlocks()
	g := NewGuard(blocks)

	now := time.Now()
	g.now = func() time.Time { return now }

	for i := 0; i < BlockAfter-1; i++ {
		if _, err := g.RecordFailure(t.Context(), "1.2.3.4", "user@example.com"); err != nil {
			t.Fatalf("failure %d: %v", i, err)
		}
	}

	// Step past the window: the count starts again, so an occasional fumble
	// spread over hours never accumulates into a permanent lockout.
	now = now.Add(Window + time.Minute)
	d, err := g.RecordFailure(t.Context(), "1.2.3.4", "user@example.com")
	if err != nil {
		t.Fatalf("failure after window: %v", err)
	}
	if d.Failures != 1 {
		t.Fatalf("failures = %d after the window elapsed, want 1", d.Failures)
	}
	if d.Blocked {
		t.Fatal("blocked despite the earlier failures having expired")
	}
}

// Identifiers are normalised, or the same account under different casing would
// get a separate allowance and multiply the effective limit.
func TestIdentifierIsNormalized(t *testing.T) {
	blocks := newFakeBlocks()
	g := NewGuard(blocks)

	if _, err := g.RecordFailure(t.Context(), "1.2.3.4", "User@Example.com "); err != nil {
		t.Fatalf("failure: %v", err)
	}
	d, err := g.Check(t.Context(), "1.2.3.4", "user@example.com")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if d.Failures != 1 {
		t.Fatalf("failures = %d, want 1: casing should not create a second bucket", d.Failures)
	}
}
