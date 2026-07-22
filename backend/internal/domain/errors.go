package domain

import "errors"

// Sentinel errors returned by the domain and application layers. The transport
// layer maps these to HTTP status codes.
var (
	ErrNotFound            = errors.New("resource not found")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrUnsupportedPlatform = errors.New("unsupported dsp platform")

	// ErrInvalidCredentials is the single answer to every failed sign-in,
	// whether the address is unknown, the password is wrong, or the account has
	// no password set. Distinguishing them in a response tells an attacker which
	// addresses are registered.
	ErrInvalidCredentials = errors.New("invalid email or password")

	// ErrEmailTaken is returned when a signup targets an address that already
	// exists, and is surfaced to the client so it can point the user at sign-in
	// rather than silently doing nothing.
	//
	// This deliberately makes signup an account-existence oracle. The trade is
	// accepted for usability, and is compensated for by rate limiting the signup
	// route per IP; password reset keeps its non-committal response so it does
	// not become a second, cheaper oracle.
	ErrEmailTaken = errors.New("email already registered")

	// ErrEmailNotVerified gates actions that require a proven address, notably
	// linking a federated identity to a local account.
	ErrEmailNotVerified = errors.New("email address is not verified")

	// ErrSessionInvalid covers an expired, revoked or unknown session.
	ErrSessionInvalid = errors.New("session is no longer valid")

	// ErrTokenReused signals that an already-rotated refresh token was
	// presented, which means the value leaked. The handler revokes the whole
	// session in response rather than merely rejecting the request.
	ErrTokenReused = errors.New("refresh token has already been used")

	// ErrTokenInvalid covers an unknown, expired or already-consumed
	// verification or password-reset token.
	ErrTokenInvalid = errors.New("token is invalid or expired")
)
