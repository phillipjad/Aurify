# 0012 — Warn-then-block lockout on the (IP, address) pair

- Status: Accepted, with an open question (see [Clearing a lockout](#clearing-a-lockout-tbd))
- Date: 2026-07-22

## Context

Aurify's authentication endpoints are unauthenticated and enumerable by nature.
Two of them invite abuse in specific ways:

- **Signup reports whether an address is already registered**, so a user who
  mistyped an address learns why no email arrived rather than being met with
  silence. That is a deliberate usability decision (see
  [ADR 0011](0011-authentication-and-sessions.md)), and it makes signup an
  account-existence oracle.
- **Sign-in** is the standing target for credential stuffing and password
  spraying.

A plain sliding-window rate limit slows both down but never stops them. An
attacker willing to wait gets unlimited attempts, just slowly.

## Decision

Failed authentication is tracked against the **(ip, identifier) pair**, where
identifier is the normalised email address, or empty for token-based routes
where the source address is the only thing to key on.

| Failures in 15 minutes | Effect |
|---|---|
| 5 | The response warns how many attempts remain and gives the support address. |
| 10 | The pair is **permanently** blocked. Further requests get 403 and the support address. |

The rolling count lives in process memory. The permanent block is a row in
`auth_blocks` in PostgreSQL.

### Why the pair, and not either half

This is the load-bearing part of the design.

- Keying on the **address alone** would let anyone lock any user out of their
  own account by deliberately failing sign-in against it. That converts a
  security control into a denial-of-service weapon, and is a well-known own goal.
- Keying on the **IP alone** would take out every user behind a shared or
  carrier-grade NAT address along with the attacker.
- Keying on the **pair** means a blocked attacker cannot reach that one account
  from that one source, while the owner is unaffected from their own address and
  other users behind the same address are unaffected too.

### Why the block is persisted but the counter is not

An in-memory block would silently evaporate on every deploy, making "permanent"
untrue in exactly the situation where it matters. The rolling counter is
deliberately *not* persisted: losing it on restart costs an attacker at most one
extra window, and persisting it would mean a database write on every failed
password attempt.

### What does not count as a failure

- **A duplicate signup.** Re-registering an address you own is not an attack,
  and locking someone out for it would be hostile.
- **Signing in to an unverified account.** The password was correct. Counting it
  would lock out the one user who most needs to retry after clicking their
  verification link.

### A successful sign-in does not clear a block

It clears the rolling counter only. If a correct password lifted a permanent
block, an attacker who eventually guessed one would erase the lockout and the
evidence along with it.

## Clearing a lockout (TBD)

**There is currently no way to lift a block from inside the product.** The only
resolution is an operator deleting the row; the `DeleteAuthBlock` query exists
solely for that and is never called from the request path.

The intended path is an **admin portal**, which is not yet designed or built.
Open questions deferred to that work:

- Who is authorised to unblock, and how are admins themselves authenticated?
- Is unblocking audited, and for how long is the record kept?
- Should blocks expire after a long interval (say 30 days) as a safety valve, or
  stay genuinely permanent?
- Should a blocked user be able to self-serve through a verified email
  challenge, trading some of the control's value for far less support load?
- What is the retention policy for `auth_blocks`?

Until that exists, users hit a hard wall and the support address is the only way
back. **This gap should be closed before any real user volume.**

## Consequences

**Good**

- Bounded rather than merely slowed: an attacker gets ten attempts per account
  per source, ever, instead of ten per window forever.
- The warning at five means the block is never a surprise.
- The pair scoping avoids the account-lockout denial of service.

**Bad, accepted**

- A legitimate user who fumbles ten times from one address is locked out of that
  account from that address until an operator intervenes. On a residential
  address that rotates they recover on their own; on a static one they do not.
- Support load lands on a human with no tooling until the admin portal exists.
  This is the main cost and the reason the TBD above matters.
- A distributed attacker rotating source addresses still gets ten attempts per
  address. This raises the cost of credential stuffing but does not eliminate
  it, and is not a substitute for password quality or for eventual MFA.
- `auth_blocks` grows without bound and an attacker can drive that growth
  deliberately. The rows are tiny, but this needs a retention answer.

**Operational**

- The rolling counter is per-process, so behind N instances an attacker gets up
  to N times the failures before tripping the block. The persisted block is
  shared, so it takes effect everywhere once written. Move the counter to a
  shared store if Aurify is scaled out.
- `AURIFY_SUPPORT_EMAIL` must point at a real, monitored address before launch.
  The default is a placeholder.

## References

- [ADR 0011](0011-authentication-and-sessions.md) — the surrounding authentication design.
