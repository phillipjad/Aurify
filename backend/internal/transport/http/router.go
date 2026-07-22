// Package http builds the mux router for the Aurify API and wires it to the
// application layer. Routes are grouped so the write side (commands) and read
// side (queries) read clearly, and each is documented for OpenAPI generation.
package http

import (
	"github.com/fgrzl/claims"

	"context"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
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
	// Verifier validates access tokens. Verification is stateless by design, so
	// an authenticated request costs no database round trip; the price is that a
	// revoked session keeps working until its access token expires.
	Verifier *auth.Verifier
	Cookies  *handlers.CookieWriter
	Throttle *handlers.Throttle
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
	mux.UseAuthentication(router, mux.WithAuthValidator(func(token string) (claims.Principal, error) {
		verified, err := deps.Verifier.Verify(token)
		if err != nil {
			return nil, err
		}
		set := claims.NewClaimsSet(verified.Subject)
		set.Set("sid", verified.SessionID)
		return claims.NewPrincipal(set), nil
	}))

	// CSRF for cookie-authenticated mutations. Refresh is exempt: the refresh
	// cookie is itself the credential, a cross-site caller cannot read the
	// response, and requiring the header would break silent refresh on a cold
	// page load when the client holds no token yet.
	router.Use(handlers.CSRFMiddleware(deps.Cookies, map[string]bool{
		"/api/v1/auth/refresh": true,
	}))

	dspAuth := handlers.NewAuth(application, providers)
	sessions := handlers.NewSessions(application, deps.Cookies, deps.Throttle, deps.Verifier)
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

		api.GET("/auth/session", sessions.Session).
			WithOperationID("getSession").
			WithSummary("Describe the signed-in user").
			WithOKResponse(dto.SessionResponse{}).
			WithResponse(401, mux.ProblemDetails{})

		// ---- write side (commands) ----
		api.GET("/auth/{platform}/login", dspAuth.Login).
			AllowAnonymous().
			WithOperationID("dspLogin").
			WithSummary("Get the OAuth authorization URL for a DSP").
			WithPathParam("platform", "DSP platform: spotify, apple_music, youtube_music", "spotify")

		api.GET("/auth/{platform}/callback", dspAuth.Callback).
			AllowAnonymous().
			WithOperationID("dspCallback").
			WithSummary("OAuth callback that links a DSP account to the user").
			WithPathParam("platform", "DSP platform: spotify, apple_music, youtube_music", "spotify").
			WithRequiredQueryParam("code", "OAuth authorization code from the provider", "AQD...")

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
