// Package dspconn resolves a user's connection to a DSP, renewing and storing
// expired credentials on the way through.
//
// It exists because reading playlists and generating a cover both need the same
// three steps — load the user, find the connection, resolve the provider — and
// both need the refresh that keeps the stored token current. Duplicated, the
// refresh would have to be remembered twice.
package dspconn

import (
	"context"
	"fmt"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// Resolver hands out a ready-to-use provider and connection for a user.
type Resolver struct {
	users     ports.UserRepository
	providers ports.DSPRegistry
}

// NewResolver constructs a Resolver.
func NewResolver(users ports.UserRepository, providers ports.DSPRegistry) *Resolver {
	return &Resolver{users: users, providers: providers}
}

// Resolve returns the provider for a platform and the user's credentials for it.
//
// Credentials are refreshed first, and a refreshed set is written back before it
// is used, so the stored token matches the one in play. A failed write is fatal:
// carrying on would spend a refresh whose result is lost, which is the behaviour
// this replaced.
func (r *Resolver) Resolve(
	ctx context.Context,
	userID string,
	platform domain.DSPPlatform,
) (ports.DSPProvider, domain.DSPConnection, error) {
	user, err := r.users.FindByID(ctx, userID)
	if err != nil {
		return nil, domain.DSPConnection{}, err
	}

	conn, ok := user.Connections[platform]
	if !ok {
		return nil, domain.DSPConnection{}, fmt.Errorf(
			"%w: user has no %s connection", domain.ErrUnauthorized, platform,
		)
	}

	provider, err := r.providers.Get(platform)
	if err != nil {
		return nil, domain.DSPConnection{}, err
	}

	refreshed, changed, err := provider.RefreshConnection(ctx, conn)
	if err != nil {
		return nil, domain.DSPConnection{}, err
	}
	if changed {
		user.Connections[platform] = refreshed
		if err := r.users.Save(ctx, user); err != nil {
			return nil, domain.DSPConnection{}, err
		}
	}

	return provider, refreshed, nil
}
