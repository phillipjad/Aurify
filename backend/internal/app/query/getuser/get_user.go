// Package getuser reads the currently signed-in user.
package getuser

import (
	"context"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Query identifies the user to read.
type Query struct {
	UserID string
}

// Handler executes the GetUser query.
type Handler struct {
	users ports.UserRepository
}

// NewHandler constructs a GetUser handler.
func NewHandler(users ports.UserRepository) *Handler {
	return &Handler{users: users}
}

// Handle returns the user record behind an authenticated session.
//
// Reading the user rather than trusting the access token's claims is what makes
// profile changes and email verification take effect immediately, instead of
// only after the token happens to be refreshed.
func (h *Handler) Handle(ctx context.Context, q Query) (*domain.User, error) {
	if q.UserID == "" {
		return nil, domain.ErrUnauthorized
	}
	return h.users.FindByID(ctx, q.UserID)
}
