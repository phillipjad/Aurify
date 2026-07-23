-- +goose Up
-- Permanent lockout records for repeated failed authentication.
-- See docs/adr/0012-account-lockout-policy.md.
--
-- The key is the (ip, identifier) pair rather than either alone, and that
-- choice is load-bearing:
--   - Blocking an address alone would let anyone lock any user out of their own
--     account by deliberately failing sign-in against it, which turns the
--     control into a denial-of-service tool.
--   - Blocking an IP alone would take out every user behind a shared or
--     carrier-grade NAT address along with the attacker.
-- Pairing them means a blocked attacker cannot reach that one account from that
-- one source, and nobody else is affected.
--
-- These rows are deliberately permanent. Nothing in the application clears
-- them; removal is a manual, operator-side action pending the admin portal.
CREATE TABLE auth_blocks (
    ip         TEXT NOT NULL,
    -- Normalised email when the route has one, empty for token-based routes
    -- where the source address is the only thing to key on.
    identifier TEXT NOT NULL DEFAULT '',
    blocked_at TIMESTAMPTZ NOT NULL,
    failures   INT NOT NULL DEFAULT 0,
    reason     TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (ip, identifier)
);

-- Supports the future admin view of "who is blocked, most recent first".
CREATE INDEX idx_auth_blocks_blocked_at ON auth_blocks (blocked_at DESC);

-- +goose Down
DROP TABLE auth_blocks;
