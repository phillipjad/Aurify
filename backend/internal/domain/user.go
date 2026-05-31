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
	ID          string                        `bson:"_id,omitempty"`
	Email       string                        `bson:"email"`
	DisplayName string                        `bson:"display_name"`
	Connections map[DSPPlatform]DSPConnection `bson:"connections"`
	CreatedAt   time.Time                     `bson:"created_at"`
	UpdatedAt   time.Time                     `bson:"updated_at"`
}

// DSPConnection holds the OAuth credentials and state for one linked DSP
// account. Tokens are stored encrypted-at-rest in a production deployment;
// the scaffold stores them as-is (see docs/adr/0005-dsp-provider-abstraction.md).
type DSPConnection struct {
	Platform       DSPPlatform `bson:"platform"`
	ProviderUserID string      `bson:"provider_user_id"`
	AccessToken    string      `bson:"access_token"`
	RefreshToken   string      `bson:"refresh_token"`
	ExpiresAt      time.Time   `bson:"expires_at"`
	Scopes         []string    `bson:"scopes"`
}
