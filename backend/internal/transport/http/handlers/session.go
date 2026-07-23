package handlers

import (
	"errors"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/command/refreshsession"
	"github.com/phillipjad/aurify/backend/internal/app/command/requestpasswordreset"
	"github.com/phillipjad/aurify/backend/internal/app/command/resetpassword"
	"github.com/phillipjad/aurify/backend/internal/app/command/signin"
	"github.com/phillipjad/aurify/backend/internal/app/command/signout"
	"github.com/phillipjad/aurify/backend/internal/app/command/signup"
	"github.com/phillipjad/aurify/backend/internal/app/command/verifyemail"
	"github.com/phillipjad/aurify/backend/internal/app/lockout"
	"github.com/phillipjad/aurify/backend/internal/app/query/getuser"
	"github.com/phillipjad/aurify/backend/internal/app/sessions"
	"github.com/phillipjad/aurify/backend/internal/domain"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
	"github.com/phillipjad/aurify/backend/internal/transport/http/dto"
)

// Sessions handles account authentication: registration, sign-in, token
// refresh, sign-out and the emailed verification and reset flows.
//
// It is separate from Auth, which handles the DSP OAuth flow. The two are not
// the same concern: one establishes who the user is, the other grants Aurify
// access to a music library.
type Sessions struct {
	app          *app.App
	cookies      *CookieWriter
	guard        *lockout.Guard
	verifier     *auth.Verifier
	supportEmail string
}

// NewSessions constructs the session handler.
func NewSessions(
	a *app.App,
	cookies *CookieWriter,
	guard *lockout.Guard,
	verifier *auth.Verifier,
	supportEmail string,
) *Sessions {
	return &Sessions{app: a, cookies: cookies, guard: guard, verifier: verifier, supportEmail: supportEmail}
}

// guardOrReject consults the lockout policy before any work is done. It returns
// false when the request must not proceed, having already written the response.
//
// Checking up front matters: a blocked pair should never reach password hashing
// or the database, both so the block is cheap to enforce and so a locked-out
// attacker learns nothing from timing.
func (h *Sessions) guardOrReject(c mux.RouteContext, ip, identifier string) bool {
	decision, err := h.guard.Check(c, ip, identifier)
	if err != nil {
		c.ServerError("internal error", err.Error())
		return false
	}
	if decision.Blocked {
		respondBlocked(c, h.supportEmail)
		return false
	}
	return true
}

// recordFailure counts a failed attempt and writes the response, appending the
// warning once the user is close to a permanent block.
func (h *Sessions) recordFailure(c mux.RouteContext, ip, identifier string, cause error) {
	decision, err := h.guard.RecordFailure(c, ip, identifier)
	if err != nil {
		c.ServerError("internal error", err.Error())
		return
	}
	if decision.Blocked {
		respondBlocked(c, h.supportEmail)
		return
	}
	if note := warning(decision, h.supportEmail); note != "" {
		c.JSON(401, map[string]string{
			"title":  "Sign-in failed",
			"detail": "Invalid email or password." + note,
		})
		return
	}
	respondError(c, cause)
}

// SignUp registers an account and sends a verification email.
// POST /api/v1/auth/signup
//
// Guarded per (IP, address). This endpoint reports whether an address is
// already registered, which is a deliberate usability trade, and the lockout is
// what keeps that disclosure from scaling into bulk enumeration of the user
// table.
//
// A duplicate address does not count as a failure: someone re-registering an
// address they own is not attacking anything, and locking them out for it would
// be hostile. Only malformed or rejected submissions count.
func (h *Sessions) SignUp(c mux.RouteContext) {
	var req dto.SignUpRequest
	if err := c.Bind(&req); err != nil {
		c.BadRequest("invalid request", "Body must be valid JSON.")
		return
	}
	ip := clientIP(c.Request())
	if !h.guardOrReject(c, ip, req.Email) {
		return
	}

	if err := h.app.Commands.SignUp.Handle(c, signup.Command{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
	}); err != nil {
		if !errors.Is(err, domain.ErrEmailTaken) {
			if _, rerr := h.guard.RecordFailure(c, ip, req.Email); rerr != nil {
				c.ServerError("internal error", rerr.Error())
				return
			}
		}
		respondError(c, err)
		return
	}
	h.guard.Reset(ip, req.Email)
	c.Created(dto.MessageResponse{Message: "Check your email for a verification link."})
}

// SignIn authenticates and starts a session.
// POST /api/v1/auth/signin
func (h *Sessions) SignIn(c mux.RouteContext) {
	var req dto.SignInRequest
	if err := c.Bind(&req); err != nil {
		c.BadRequest("invalid request", "Body must be valid JSON.")
		return
	}
	// Guarded on the (IP, address) pair, so neither credential stuffing against
	// one account nor spraying across many is cheap.
	ip := clientIP(c.Request())
	if !h.guardOrReject(c, ip, req.Email) {
		return
	}

	tokens, err := h.app.Commands.SignIn.Handle(c, signin.Command{
		Email:     req.Email,
		Password:  req.Password,
		UserAgent: c.Request().UserAgent(),
		IP:        ip,
	})
	if err != nil {
		// An unverified account is not a failed credential: the password was
		// correct. Counting it would lock out the one user who most needs to
		// retry after clicking their verification link.
		if errors.Is(err, domain.ErrEmailNotVerified) {
			respondError(c, err)
			return
		}
		h.recordFailure(c, ip, req.Email, err)
		return
	}
	h.guard.Reset(ip, req.Email)
	h.respondWithSession(c, tokens)
}

// Refresh rotates the refresh token and reissues the pair.
// POST /api/v1/auth/refresh
//
// Deliberately not CSRF-protected: the refresh cookie is the credential, and a
// cross-site caller cannot read the response, so forcing a refresh achieves
// nothing beyond rotating the victim's own token. Requiring the CSRF header
// here would instead break the silent-refresh path after a cold page load,
// where the client has no token in memory yet.
func (h *Sessions) Refresh(c mux.RouteContext) {
	presented := h.cookies.RefreshToken(c)
	if presented == "" {
		c.Unauthorized()
		return
	}

	tokens, err := h.app.Commands.RefreshSession.Handle(c, refreshsession.Command{
		RefreshToken: presented,
	})
	if err != nil {
		// The session is gone (expired, revoked, or just killed for token
		// reuse), so drop the cookies rather than leaving the browser to keep
		// presenting credentials that will never work again.
		h.cookies.Clear(c)
		respondError(c, err)
		return
	}
	h.respondWithSession(c, tokens)
}

// SignOut revokes the current session.
// POST /api/v1/auth/signout
func (h *Sessions) SignOut(c mux.RouteContext) {
	// Best effort: read the session id from the access token if one is present.
	// Sign-out must succeed even with an expired token, or a user whose access
	// token lapsed could never explicitly sign out.
	sessionID := ""
	if token := h.cookies.AccessToken(c); token != "" {
		if claims, err := h.verifier.Verify(token); err == nil {
			sessionID = claims.SessionID
		}
	}

	if err := h.app.Commands.SignOut.Handle(c, signout.Command{SessionID: sessionID}); err != nil {
		respondError(c, err)
		return
	}
	h.cookies.Clear(c)
	c.NoContent()
}

// VerifyEmail consumes a verification link.
// POST /api/v1/auth/verify-email
func (h *Sessions) VerifyEmail(c mux.RouteContext) {
	var req dto.TokenRequest
	if err := c.Bind(&req); err != nil {
		c.BadRequest("invalid request", "Body must be valid JSON.")
		return
	}
	ip := clientIP(c.Request())
	if !h.guardOrReject(c, ip, "") {
		return
	}

	if err := h.app.Commands.VerifyEmail.Handle(c, verifyemail.Command{Token: req.Token}); err != nil {
		// A bad token here is a guess at a credential, so it counts.
		h.recordFailure(c, ip, "", err)
		return
	}
	h.guard.Reset(ip, "")
	c.OK(dto.MessageResponse{Message: "Email verified. You can sign in now."})
}

// ForgotPassword sends a reset link.
// POST /api/v1/auth/password/forgot
//
// Always answers the same way, whether or not the address is registered. Signup
// already discloses account existence, but making this endpoint disclose it too
// would hand attackers a second oracle that costs nothing and sends mail as a
// side effect.
func (h *Sessions) ForgotPassword(c mux.RouteContext) {
	var req dto.ForgotPasswordRequest
	if err := c.Bind(&req); err != nil {
		c.BadRequest("invalid request", "Body must be valid JSON.")
		return
	}
	if !h.guardOrReject(c, clientIP(c.Request()), req.Email) {
		return
	}

	if err := h.app.Commands.RequestPasswordReset.Handle(c, requestpasswordreset.Command{
		Email: req.Email,
	}); err != nil {
		respondError(c, err)
		return
	}
	c.OK(dto.MessageResponse{Message: "If that address has an account, a reset link is on its way."})
}

// ResetPassword sets a new password from a reset token.
// POST /api/v1/auth/password/reset
func (h *Sessions) ResetPassword(c mux.RouteContext) {
	var req dto.ResetPasswordRequest
	if err := c.Bind(&req); err != nil {
		c.BadRequest("invalid request", "Body must be valid JSON.")
		return
	}
	ip := clientIP(c.Request())
	if !h.guardOrReject(c, ip, "") {
		return
	}

	if err := h.app.Commands.ResetPassword.Handle(c, resetpassword.Command{
		Token:       req.Token,
		NewPassword: req.NewPassword,
	}); err != nil {
		h.recordFailure(c, ip, "", err)
		return
	}
	h.guard.Reset(ip, "")
	// Every session was revoked, so the browser's cookies are now worthless.
	h.cookies.Clear(c)
	c.OK(dto.MessageResponse{Message: "Password updated. Sign in with your new password."})
}

// Session returns the signed-in user.
// GET /api/v1/auth/session
func (h *Sessions) Session(c mux.RouteContext) {
	userID := currentUser(c)
	if userID == "" {
		c.Unauthorized()
		return
	}

	user, err := h.app.Queries.GetUser.Handle(c, getuser.Query{UserID: userID})
	if err != nil {
		respondError(c, err)
		return
	}
	c.OK(dto.SessionResponse{
		UserID:        user.ID,
		Email:         user.Email,
		DisplayName:   user.DisplayName,
		EmailVerified: user.EmailVerified,
	})
}

// respondWithSession writes the cookie set and echoes the session plus the CSRF
// token the client must send on subsequent state-changing requests.
func (h *Sessions) respondWithSession(c mux.RouteContext, tokens sessions.Tokens) {
	csrf, err := h.cookies.Issue(c, tokens)
	if err != nil {
		c.ServerError("internal error", err.Error())
		return
	}

	userID := ""
	if claims, verr := h.verifier.Verify(tokens.AccessToken); verr == nil {
		userID = claims.Subject
	}

	response := dto.SessionResponse{UserID: userID, CSRFToken: csrf}
	if user, uerr := h.app.Queries.GetUser.Handle(c, getuser.Query{UserID: userID}); uerr == nil {
		response.Email = user.Email
		response.DisplayName = user.DisplayName
		response.EmailVerified = user.EmailVerified
	}
	c.OK(response)
}
