// Package connectdsp handles linking a DSP account to an Aurify user after the
// provider's OAuth callback.
package connectdsp

import (
	"context"
	"time"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Command links a DSP account to a user using an OAuth authorization code.
type Command struct {
	UserID   string
	Platform domain.DSPPlatform
	Code     string
}

// Handler executes the ConnectDSP command.
type Handler struct {
	users     ports.UserRepository
	providers ports.DSPRegistry
}

// NewHandler constructs a ConnectDSP handler.
func NewHandler(users ports.UserRepository, providers ports.DSPRegistry) *Handler {
	return &Handler{users: users, providers: providers}
}

// Handle exchanges the OAuth code for tokens and stores the connection on the
// user.
func (h *Handler) Handle(ctx context.Context, cmd Command) error {
	provider, err := h.providers.Get(cmd.Platform)
	if err != nil {
		return err
	}

	conn, err := provider.Exchange(ctx, cmd.Code)
	if err != nil {
		return err
	}

	user, err := h.users.FindByID(ctx, cmd.UserID)
	if err != nil {
		return err
	}

	if user.Connections == nil {
		user.Connections = make(map[domain.DSPPlatform]domain.DSPConnection)
	}
	user.Connections[cmd.Platform] = conn
	user.UpdatedAt = time.Now().UTC()

	return h.users.Save(ctx, user)
}
