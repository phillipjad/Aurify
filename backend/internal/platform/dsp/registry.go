// Package dsp provides the registry that resolves DSP providers, plus the
// shared OAuth config type. Concrete providers live in sub-packages
// (spotify, applemusic, youtubemusic). The registry deliberately does not
// import those sub-packages: providers are constructed in cmd/api and passed
// in, which keeps the dependency graph acyclic.
package dsp

import (
	"fmt"

	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/domain"
)

// OAuthConfig is the per-provider OAuth client configuration consumed by the
// provider sub-packages.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// Registry resolves a ports.DSPProvider by platform. It satisfies
// ports.DSPRegistry.
type Registry struct {
	providers map[domain.DSPPlatform]ports.DSPProvider
}

// Compile-time guarantee that Registry implements the port.
var _ ports.DSPRegistry = (*Registry)(nil)

// NewRegistry builds a registry from the supplied providers, keyed by platform.
// Nil providers are ignored.
func NewRegistry(providers ...ports.DSPProvider) *Registry {
	m := make(map[domain.DSPPlatform]ports.DSPProvider, len(providers))
	for _, p := range providers {
		if p == nil {
			continue
		}
		m[p.Platform()] = p
	}
	return &Registry{providers: m}
}

// Get returns the provider for a platform, or ErrUnsupportedPlatform.
func (r *Registry) Get(platform domain.DSPPlatform) (ports.DSPProvider, error) {
	p, ok := r.providers[platform]
	if !ok {
		return nil, fmt.Errorf("%w: %q", domain.ErrUnsupportedPlatform, platform)
	}
	return p, nil
}

// Platforms lists the registered platforms.
func (r *Registry) Platforms() []domain.DSPPlatform {
	out := make([]domain.DSPPlatform, 0, len(r.providers))
	for p := range r.providers {
		out = append(out, p)
	}
	return out
}
