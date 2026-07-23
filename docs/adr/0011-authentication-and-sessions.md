# 0011 — Hand-built authentication: Ed25519 access tokens + opaque refresh tokens

- Status: Accepted (amended 2026-07-23, see [Amendment](#amendment-2026-07-23-revocation-is-now-immediate))
- Date: 2026-07-22

## Context

Aurify has had no authentication. Every handler read the caller's identity from
an `X-User-ID` request header, which is a complete authentication bypass: any
client could name any user and be believed. The frontend sent a hardcoded
`demo-user`. This was always marked `SCAFFOLD:` and always had to go before
anything real shipped.

We need email/password sign-in and Google sign-in, with sessions that survive a
restart, can be revoked centrally, and are delivered by cookie.

Two constraints shaped the decision:

1. **Security is the priority.** The design is judged on how it fails, not on
   how little code it takes.
2. **We deliberately want to build this rather than adopt a turnkey solution.**
   Understanding the session and token design is an explicit goal of the work.

`fgrzl/mux` ships a batteries-included authentication middleware with its own
CSRF, rate limiting, token provider and revocation hooks, and `fgrzl/claims`
ships JWT signers. Using all of it would have been the shortest path.

## Decision

### Token model: short access JWT + opaque refresh token

An **Ed25519-signed access token** (15 minutes) is verified statelessly on every
request. A **256-bit opaque refresh token** (30-day idle, 90-day absolute) is
stored in PostgreSQL as a SHA-256 digest and rotated on every use.

The original proposal was "JWTs stored centrally in Postgres". That is
self-defeating: if every request validates by looking the token up in the
database, you pay the round trip that JWTs exist to avoid *and* take on the JWT
attack surface, for no gain over an opaque token. What "store it in the
database" actually buys is **revocability**, and the split above gets that from
the refresh token while keeping the hot path stateless.

The cost is explicit: because access tokens are verified without a database
read, **a revoked session keeps working until its access token expires**. The
15-minute TTL is that exposure window, and it is the reason the TTL is short.

### We build the logic; mux supplies only the mounting point

`mux.UseAuthentication` is used solely to register middleware, populate
`c.User()` and preserve the existing `AllowAnonymous()` route bookkeeping. The
validator, the session store, refresh rotation, reuse detection, CSRF and the
lockout policy are all ours.

The cryptographic primitives are **not** ours: entropy from `crypto/rand`,
digests from `crypto/sha256`, comparisons via `crypto/subtle`, signatures from
`crypto/ed25519`, password hashing from `golang.org/x/crypto/argon2`. The line
is deliberate. Composing primitives is the design work worth doing by hand;
reimplementing them is how you get a CVE, and it teaches nothing the composition
does not.

### JWT verification rules

The verifier pins the algorithm. The `alg` header is read for exactly one
purpose: to reject anything that is not `EdDSA`. It never selects a key or an
algorithm. This closes both classic JWT breaks in one comparison: `alg: none`,
which asks the verifier to skip the signature, and algorithm confusion, where an
attacker signs an HS256 token using our published Ed25519 public key as the HMAC
secret.

The signature is verified **before** the payload is parsed, so no
attacker-controlled field is acted on before the token is known to be ours. A
token with no `exp` is rejected even if we signed it, because a token that never
expires is a permanent credential. `iss`, `aud`, `nbf` and `iat` are all checked.

### Passwords

Argon2id at the OWASP minimum (m=19456 KiB, t=2, p=1), stored in PHC string
form so the parameters travel with the hash and can be raised later; sign-in
upgrades a weaker hash while it holds the plaintext.

Sign-in returns one shared error for every failure and performs an Argon2id
derivation even when the address is unknown. The shared error hides account
existence from the response body; the decoy hash hides it from the response
time, since a real verification costs ~19 MiB of work that an early return
would skip.

### Cookies

| Cookie | Prefix | Path | Purpose |
|---|---|---|---|
| access | `__Host-` | `/` | Sent on every request |
| refresh | `__Secure-` | `/api/v1/auth` | Only sent where it is used |
| CSRF | none | `/` | Readable by script, echoed in a header |

`__Host-` pins a cookie to exactly this origin so a subdomain cannot overwrite
it, but it requires `Path=/`. The refresh cookie is deliberately scoped to a
narrow path so the long-lived credential is not attached to every request, so it
cannot use `__Host-` and uses `__Secure-` instead.

**`SameSite` is `Lax`, not `Strict`.** mux defaults to `Strict`, which would
silently break Google sign-in: a `Strict` cookie is not sent on the cross-site
top-level navigation that returns from the provider, so the `state`, `nonce` and
PKCE verifier would be missing at the callback.

### Refresh rotation with reuse detection

Every refresh retires the presented token and issues a replacement. Presenting a
token that was already rotated away means the value leaked, so the **entire
session is revoked**, not merely the request rejected. The legitimate holder has
to sign in again, but the thief loses access too, and we cannot tell which of
them is making the request.

The `used_at IS NULL` guard lives in the SQL `UPDATE`, so when two refreshes
race, exactly one wins and the loser is deterministically treated as reuse,
rather than the outcome depending on read-then-write ordering in Go.

### Federated identity is separate from DSP connections

`user_identities` is a different table from `dsp_connections`. The latter grants
data access to a music library; the former asserts who the user is. Sharing one
table would let a YouTube Music data connection imply a login identity, which is
a privilege escalation. Google's `sub` is the key, never the email address,
which can be changed or reassigned.

### Account linking requires a verified address

A federated identity is auto-linked to a local account only when the provider
asserts `email_verified` **and** the local account is already verified.
Otherwise an attacker could pre-register a victim's address, never verify it,
and inherit the account the first time the victim signs in with Google. This is
why sign-in requires a verified email at all.

### Signup discloses whether an address is registered

Signup answers "an account already exists, sign in instead" rather than
silently succeeding. This is a usability decision that knowingly makes signup an
account-existence oracle. It is compensated for by the lockout policy in
[ADR 0012](0012-account-lockout-policy.md), by never modifying the existing
account, and by keeping password reset non-committal so it does not become a
second and cheaper oracle.

## Consequences

**Good**

- The hot path costs no database round trip.
- Revocation is real, and token theft is *detectable* rather than merely
  survivable.
- No third-party auth dependency to track for CVEs; the primitives are stdlib
  and `x/crypto`.
- The `X-User-ID` bypass is gone.

**Bad, accepted**

- A revoked session survives up to 15 minutes. Closing that fully would mean a
  database read per request, which is the design we rejected.
- More code to own and test than adopting mux's built-in auth wholesale. The
  negative-path tests (`alg: none`, algorithm confusion, tampered payload,
  missing expiry, wrong issuer, refresh reuse) are the mitigation and are not
  optional.
- Hand-written JWT verification is a place where a future edit can quietly
  introduce a serious bug. The pinned algorithm and the tests exist to make that
  loud.

**Operational**

- `AURIFY_AUTH_SIGNING_KEY` must be set in production. When unset the API
  generates an ephemeral key and warns: sessions then die on restart, and two
  instances reject each other's tokens.
- `AURIFY_SMTP_HOST` must be set, or verification and reset links are written to
  the log instead of sent.

## Deferred

- **MFA.** Not designed. The session model does not preclude it.
- **Immediate revocation** of access tokens, which would need a shared denylist
  checked per request.
- **Encryption at rest for DSP tokens**, still stored as-is per
  [ADR 0005](0005-dsp-provider-abstraction.md).

## References

- [ADR 0007](0007-fgrzl-mux-web-framework.md) — the framework whose auth hook we mount into.
- [ADR 0012](0012-account-lockout-policy.md) — the failed-authentication lockout.

## Amendment (2026-07-23): revocation is now immediate

This ADR originally accepted that "a revoked session keeps working until its
access token expires", with the 15-minute TTL as the exposure window. That has
been reversed: every authenticated request now checks the session against the
database, so sign-out and refresh-reuse revocation take effect at once.

The original reasoning was weaker than it looked. It traded immediate revocation
for avoiding a database round trip, but **every protected endpoint in this API
already queries PostgreSQL** (covers list/get/delete, session, playlists). The
check adds one indexed primary-key lookup to requests that were already
database-bound, so the statelessness was buying far less than the argument
assumed.

What settled it was the user-facing behaviour rather than the architecture:
sign-out that does not sign the user out for up to fifteen minutes is a real
problem on a shared machine, and the same window let a stolen access token
outlive the reuse detection that was supposed to have killed it.

The access token remains a signed JWT and is still verified cryptographically
before the session lookup happens, so a forged or expired token is rejected
without touching the database. If the lookup ever becomes measurable, a
short-TTL cache in front of it bounds staleness to seconds without returning to
a fifteen-minute window.
