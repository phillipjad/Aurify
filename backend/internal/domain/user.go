package domain

import "time"

// DSPPlatform identifies a music streaming provider a user can connect.
type DSPPlatform string

// Supported DSP platforms.
const (
	PlatformSpotify      DSPPlatform = "spotify"
	PlatformAppleMusic   DSPPlatform = "apple_music"
	PlatformYouTubeMusic DSPPlatform = "youtube_music"
)

// User is a registered Aurify account. A user may link multiple DSP accounts.
type User struct {
	ID          string
	Email       string
	DisplayName string
	// EmailVerified reports whether the address on Email has been proven. It
	// gates federated account linking: linking Google to an unverified local
	// account would let an attacker pre-register a victim's address and inherit
	// the account when the victim first signs in with Google.
	//
	// It is read-only through UserRepository.Save; only the verify-email command
	// changes it (see the UpsertUser query).
	EmailVerified bool
	Connections   map[DSPPlatform]DSPConnection
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// DSPConnection holds the OAuth credentials and state for one linked DSP
// account. Tokens are stored encrypted-at-rest in a production deployment;
// the scaffold stores them as-is (see docs/adr/0005-dsp-provider-abstraction.md).
type DSPConnection struct {
	Platform       DSPPlatform
	ProviderUserID string
	AccessToken    string
	RefreshToken   string
	ExpiresAt      time.Time
	Scopes         []string
}
