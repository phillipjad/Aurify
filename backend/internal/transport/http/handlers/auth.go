package handlers

import (
	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/command/connectdsp"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Auth handles the DSP OAuth login + callback flow.
type Auth struct {
	app       *app.App
	providers ports.DSPRegistry
}

// NewAuth constructs the auth handler. It needs the provider registry directly
// because building the authorization-redirect URL is a transport concern, not a
// domain use case.
func NewAuth(a *app.App, providers ports.DSPRegistry) *Auth {
	return &Auth{app: a, providers: providers}
}

// Login returns the provider's OAuth authorization URL.
// GET /api/v1/auth/{platform}/login
func (h *Auth) Login(c mux.RouteContext) {
	platform, ok := c.Params().String("platform")
	if !ok {
		c.BadRequest("missing platform", "path parameter 'platform' is required")
		return
	}

	provider, err := h.providers.Get(domain.DSPPlatform(platform))
	if err != nil {
		respondError(c, err)
		return
	}

	// TODO: generate a cryptographically random `state`, persist it bound to the
	// session, and verify it in the callback to defend against CSRF.
	state := platform
	c.OK(map[string]string{"authUrl": provider.AuthURL(state)})
}

// Callback completes the OAuth flow and links the DSP account to the user.
// GET /api/v1/auth/{platform}/callback?code=...&state=...
func (h *Auth) Callback(c mux.RouteContext) {
	platform, ok := c.Params().String("platform")
	if !ok {
		c.BadRequest("missing platform", "path parameter 'platform' is required")
		return
	}
	code, ok := c.Query().String("code")
	if !ok {
		c.BadRequest("missing code", "query parameter 'code' is required")
		return
	}

	userID := currentUser(c)
	if userID == "" {
		c.Unauthorized()
		return
	}

	if err := h.app.Commands.ConnectDSP.Handle(c, connectdsp.Command{
		UserID:   userID,
		Platform: domain.DSPPlatform(platform),
		Code:     code,
	}); err != nil {
		respondError(c, err)
		return
	}

	c.OK(map[string]string{"status": "connected", "platform": platform})
}
