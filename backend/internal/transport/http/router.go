// Package http builds the mux router for the Aurify API and wires it to the
// application layer. Routes are grouped so the write side (commands) and read
// side (queries) read clearly, and each is documented for OpenAPI generation.
package http

import (
	"github.com/fgrzl/claims"

	"context"
	"errors"
	"net/http"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/lockout"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
	"github.com/phillipjad/aurify/backend/internal/platform/identity/google"
	"github.com/phillipjad/aurify/backend/internal/transport/http/dto"
	"github.com/phillipjad/aurify/backend/internal/transport/http/handlers"
)

// API-level OpenAPI metadata. version is supplied at call time (stamped into
// the binary); title and description are stable.
const (
	apiTitle       = "Aurify API"
	apiDescription = "Generate abstract covers from a playlist's audio features and lyric sentiment."
)

// Deps are the collaborators the router needs beyond the application layer.
type Deps struct {
	Application *app.App
	Providers   ports.DSPRegistry
	Version     string
	CORSOrigins []string
	// Ready is the readiness check behind GET /readyz; nil always reports ready.
	Ready func(context.Context) error
	// Verifier validates access tokens cryptographically, without touching the
	// database, so a forged or expired token is rejected cheaply.
	Verifier *auth.Verifier
	Cookies  *handlers.CookieWriter
	// SessionCheck confirms the verified token's session is still live. Signature
	// verification alone cannot see a revocation, so without this a signed-out
	// user keeps access until their access token expires.
	SessionCheck handlers.SessionCheck
	// Guard enforces the failed-authentication lockout policy.
	Guard *lockout.Guard
	// SupportEmail is shown to users who are close to, or already under, a
	// permanent lockout, since an operator is the only way back.
	SupportEmail string
	// Google performs the Sign in with Google handshake. It is always non-nil;
	// when no credentials are configured it reports itself disabled and the
	// routes answer 501.
	Google *google.Provider
	// AppBaseURL is the origin the federated callback redirects back to, and the
	// only origin it will redirect to.
	AppBaseURL string
}

// NewRouter builds the fully configured API router. Version is reported in the
// OpenAPI info object (defaults to "dev" when empty).
func NewRouter(deps Deps) (*mux.Router, error) {
	application := deps.Application
	providers := deps.Providers
	corsOrigins := deps.CORSOrigins
	ready := deps.Ready

	version := deps.Version
	if version == "" {
		version = "dev"
	}
	router := mux.NewRouter(
		mux.WithTitle(apiTitle),
		mux.WithVersion(version),
		mux.WithDescription(apiDescription),
	)

	mux.UseLogging(router)
	mux.UseCompression(router)
	if len(corsOrigins) > 0 {
		// X-User-ID is deliberately gone from the allowlist: it used to carry
		// the caller's identity and was a complete authentication bypass.
		mux.UseCORS(router,
			mux.WithCORSAllowedOrigins(corsOrigins...),
			mux.WithCORSAllowedMethods("GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"),
			mux.WithCORSAllowedHeaders("Content-Type", "Authorization", "X-CSRF-Token"),
			mux.WithCORSCredentials(true),
		)
	}

	// Authentication. mux supplies only the mounting point and the AllowAnonymous
	// bookkeeping; the validator below is ours, and it verifies the token rather
	// than trusting anything the client asserts.
	mux.UseAuthentication(router,
		mux.WithAuthValidator(func(token string) (claims.Principal, error) {
			verified, err := deps.Verifier.Verify(token)
			if err != nil {
				return nil, err
			}
			set := claims.NewClaimsSet(verified.Subject)
			set.Set(handlers.SessionClaim, verified.SessionID)
			return claims.NewPrincipal(set), nil
		}),
		// The middleware must be told which cookie carries the token. Its
		// default is "app_token", so without this it looks for a cookie we never
		// write and every cookie-authenticated request quietly 401s.
		mux.WithAuthAppSessionCookieName(deps.Cookies.AccessCookieName()),
		// These have to mirror how the cookie was written. On a successful
		// cookie authentication the middleware may re-issue the cookie to extend
		// it, and it applies these options when it does. Left unset it would
		// fall back to its own defaults, whose SameSite is Strict, silently
		// undoing the Lax setting that the federated sign-in callback depends on
		// (see docs/adr/0011-authentication-and-sessions.md).
		mux.WithAuthCookieOptions(
			mux.WithCookiePath("/"),
			mux.WithCookieSecure(deps.Cookies.Secure()),
			mux.WithCookieHTTPOnly(true),
			mux.WithCookieSameSite(http.SameSiteLaxMode),
		),
	)

	// Revocation, immediately after authentication so the principal is populated.
	// The token is trusted cryptographically by this point but has not been
	// checked against the session it names, which is what makes sign-out and
	// refresh-reuse revocation take effect at once instead of lagging by the
	// access-token lifetime.
	//
	// A missing check is a startup failure rather than a silently skipped
	// middleware. Every authenticated route would still answer 200 with a token
	// belonging to a revoked session, which is precisely the kind of hole nobody
	// notices until it is exploited.
	if deps.SessionCheck == nil {
		return nil, errors.New("router: SessionCheck is required, revocation cannot be optional")
	}
	router.Use(handlers.SessionRevocationMiddleware(deps.SessionCheck))

	// CSRF for cookie-authenticated mutations. Refresh is exempt: the refresh
	// cookie is itself the credential, a cross-site caller cannot read the
	// response, and requiring the header would break silent refresh on a cold
	// page load when the client holds no token yet.
	router.Use(handlers.CSRFMiddleware(deps.Cookies, map[string]bool{
		"/api/v1/auth/refresh": true,
	}))

	dspAuth := handlers.NewAuth(application, providers, deps.Cookies, deps.AppBaseURL)
	sessions := handlers.NewSessions(application, deps.Cookies, deps.Guard, deps.Verifier, deps.SupportEmail)
	federated := handlers.NewFederated(
		application, deps.Google, deps.Cookies, deps.Guard, deps.AppBaseURL, deps.SupportEmail,
	)
	playlists := handlers.NewPlaylists(application)
	covers := handlers.NewCovers(application)

	err := router.Configure(func(r *mux.Router) {
		// Kubernetes-style probes (provided by mux).
		r.Livez()
		r.ReadyzWithCheck(func(c mux.RouteContext) bool {
			if ready == nil {
				return true
			}
			return ready(c) == nil
		})

		api := r.Group("/api/v1")
		api.WithTags("aurify")

		// ---- account authentication ----
		api.POST("/auth/signup", sessions.SignUp).
			AllowAnonymous().
			WithOperationID("signUp").
			WithSummary("Register an email/password account").
			WithJSONBody(dto.SignUpRequest{}).
			WithCreatedResponse(dto.MessageResponse{}).
			WithResponse(409, mux.ProblemDetails{})

		api.POST("/auth/signin", sessions.SignIn).
			AllowAnonymous().
			WithOperationID("signIn").
			WithSummary("Authenticate and start a session").
			WithJSONBody(dto.SignInRequest{}).
			WithOKResponse(dto.SessionResponse{}).
			WithResponse(401, mux.ProblemDetails{})

		api.POST("/auth/refresh", sessions.Refresh).
			AllowAnonymous().
			WithOperationID("refreshSession").
			WithSummary("Rotate the refresh token and reissue the session").
			WithOKResponse(dto.SessionResponse{}).
			WithResponse(401, mux.ProblemDetails{})

		api.POST("/auth/signout", sessions.SignOut).
			AllowAnonymous().
			WithOperationID("signOut").
			WithSummary("Revoke the current session").
			WithNoContentResponse()

		api.POST("/auth/verify-email", sessions.VerifyEmail).
			AllowAnonymous().
			WithOperationID("verifyEmail").
			WithSummary("Confirm an email address using an emailed token").
			WithJSONBody(dto.TokenRequest{}).
			WithOKResponse(dto.MessageResponse{})

		api.POST("/auth/password/forgot", sessions.ForgotPassword).
			AllowAnonymous().
			WithOperationID("forgotPassword").
			WithSummary("Send a password reset link").
			WithJSONBody(dto.ForgotPasswordRequest{}).
			WithOKResponse(dto.MessageResponse{})

		api.POST("/auth/password/reset", sessions.ResetPassword).
			AllowAnonymous().
			WithOperationID("resetPassword").
			WithSummary("Set a new password using a reset token").
			WithJSONBody(dto.ResetPasswordRequest{}).
			WithOKResponse(dto.MessageResponse{})

		// ---- federated sign-in (Sign in with Google) ----
		//
		// Deliberately under /auth/federated/ rather than sharing the
		// /auth/{platform}/ space with the DSP routes below. The paths must not
		// collide, and the separation is the same one the schema makes between
		// user_identities and dsp_connections: a music-library connection is not
		// a login.
		api.GET("/auth/federated/google/start", federated.GoogleStart).
			AllowAnonymous().
			WithOperationID("googleSignInStart").
			WithSummary("Redirect to Google to begin federated sign-in").
			WithQueryParam("return", "Path within the app to return to afterwards", "/covers").
			WithResponse(302, nil).
			WithResponse(501, dto.MessageResponse{})

		api.GET("/auth/federated/google/callback", federated.GoogleCallback).
			AllowAnonymous().
			WithOperationID("googleSignInCallback").
			WithSummary("Complete federated sign-in and start a session").
			WithQueryParam("code", "Authorization code from Google", "4/0A...").
			WithQueryParam("state", "Opaque value echoed back by Google", "xY...").
			WithResponse(302, nil).
			WithResponse(501, dto.MessageResponse{})

		api.GET("/auth/session", sessions.Session).
			WithOperationID("getSession").
			WithSummary("Describe the signed-in user").
			WithOKResponse(dto.SessionResponse{}).
			WithResponse(401, mux.ProblemDetails{})

		// ---- write side (commands) ----
		// Both legs are top-level browser navigations, like the federated routes
		// above, so they redirect rather than answer JSON.
		//
		// Neither may be AllowAnonymous, unlike the federated routes. That flag
		// does not mean "tolerate an anonymous caller" — mux skips the
		// authentication middleware outright, so the principal is never populated
		// and c.User() is nil even when the request carries a valid session
		// cookie. Both handlers need to know which Aurify user is linking the
		// account, so with the flag set the flow can never complete: login sees
		// nobody signed in and the callback has no user to attach tokens to.
		//
		// The cost is that an unauthenticated navigation gets mux's 401 rather
		// than a redirect to sign-in. Both URLs are reached from the playlists
		// page, which is behind the session guard, so that is the rare path.
		api.GET("/auth/{platform}/login", dspAuth.Login).
			WithOperationID("dspLogin").
			WithSummary("Redirect to a DSP to begin linking the account").
			WithPathParam("platform", "DSP platform: spotify, apple_music, youtube_music", "spotify").
			WithResponse(302, nil)

		api.GET("/auth/{platform}/callback", dspAuth.Callback).
			WithOperationID("dspCallback").
			WithSummary("OAuth callback that links a DSP account to the user").
			WithPathParam("platform", "DSP platform: spotify, apple_music, youtube_music", "spotify").
			WithRequiredQueryParam("code", "OAuth authorization code from the provider", "AQD...").
			WithQueryParam("state", "Opaque value echoed back by the provider", "xY...").
			WithResponse(302, nil)

		api.POST("/covers", covers.Generate).
			WithOperationID("generateCover").
			WithSummary("Analyze a playlist and generate a cover").
			WithJSONBody(dto.GenerateCoverRequest{}).
			WithCreatedResponse(dto.CoverResponse{})

		// ---- read side (queries) ----
		api.GET("/playlists", playlists.List).
			WithOperationID("listPlaylists").
			WithSummary("List the current user's playlists for a DSP").
			WithRequiredQueryParam("platform", "DSP platform", "spotify").
			WithQueryParam("q", "Case-insensitive search over name and description", "coffee").
			WithQueryParam("sort", "Ordering: name or tracks", "name").
			WithQueryParam("limit", "Page size (default 20, max 100)", "20").
			WithQueryParam("offset", "Rows to skip", "0").
			WithOKResponse([]dto.PlaylistResponse{})

		api.GET("/covers", covers.List).
			WithOperationID("listCovers").
			WithSummary("List covers generated by the current user").
			WithQueryParam("status", "Filter to one lifecycle status (empty = all)", "ready").
			WithQueryParam("limit", "Page size (default 20, max 100)", "20").
			WithQueryParam("offset", "Rows to skip", "0").
			WithOKResponse([]dto.CoverResponse{})

		api.GET("/covers/{id}", covers.Get).
			WithOperationID("getCover").
			WithSummary("Get a generated cover by id").
			WithPathParam("id", "Cover id", "000000000000000000000000").
			WithOKResponse(dto.CoverResponse{}).
			WithResponse(404, mux.ProblemDetails{})

		api.DELETE("/covers/{id}", covers.Delete).
			WithOperationID("deleteCover").
			WithSummary("Delete a generated cover").
			WithPathParam("id", "Cover id", "000000000000000000000000").
			WithNoContentResponse().
			WithResponse(404, mux.ProblemDetails{})

		// Serve the SPA for all non-API routes. Two registrations are required:
		// "" catches the root ("/") and "/**" catches every deeper path.
		// Both must come after API routes so that specific paths take priority.
		spa := newSPAHandler()
		r.Handle("GET", "", spa).AllowAnonymous()
		r.Handle("GET", "/**", spa).AllowAnonymous()
	})
	if err != nil {
		return nil, err
	}
	return router, nil
}
