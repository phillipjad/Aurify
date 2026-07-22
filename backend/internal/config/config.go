// Package config loads runtime configuration from the environment. All
// settings have development-friendly defaults so the API can boot with zero
// configuration against a local PostgreSQL.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully resolved application configuration.
type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	LRCLibBaseURL string
	// PromptGenURL and ImageGenURL point at the local LLM sidecars. They are
	// intentionally empty by default; see docs/adr/0006-llm-sidecars.md.
	PromptGenURL string
	ImageGenURL  string
	CORSOrigins  []string
	DSP          DSPConfig
	// AppBaseURL is the public origin of the web app. It is what verification
	// and password-reset links are built against, so it must be the address the
	// user's browser can actually reach.
	AppBaseURL string
	// SupportEmail is shown to users who hit the permanent authentication
	// lockout, since only an operator can lift one.
	SupportEmail string
	Auth         AuthConfig
	SMTP         SMTPConfig
}

// AuthConfig holds the session and token settings.
type AuthConfig struct {
	// SigningKeySeed is a base64 32-byte Ed25519 seed. Empty means "generate an
	// ephemeral key at startup", which is a development convenience: every
	// restart invalidates outstanding access tokens, and a multi-instance
	// deployment would sign with keys the other instances cannot verify.
	SigningKeySeed string
	Issuer         string
	Audience       string
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	SessionTTL     time.Duration
	// CookieSecure marks auth cookies Secure. It defaults to true and should
	// only ever be disabled for local HTTP development.
	CookieSecure bool
}

// SMTPConfig holds the outbound mail relay credentials (SMTP2GO by default).
// An empty Host selects the development sender, which logs messages instead of
// delivering them.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// DSPConfig groups the OAuth client configuration for each supported DSP.
type DSPConfig struct {
	Spotify      OAuthConfig
	AppleMusic   OAuthConfig
	YouTubeMusic OAuthConfig
}

// OAuthConfig holds a single DSP's OAuth client credentials.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// Load reads configuration from environment variables, applying defaults.
func Load() Config {
	return Config{
		HTTPAddr: env("AURIFY_HTTP_ADDR", ":8080"),
		DatabaseURL: env(
			"AURIFY_DATABASE_URL",
			"postgres://aurify:aurify@localhost:5432/aurify?sslmode=disable",
		),
		LRCLibBaseURL: env("AURIFY_LRCLIB_URL", "https://lrclib.net"),
		PromptGenURL:  env("AURIFY_PROMPTGEN_URL", ""),
		ImageGenURL:   env("AURIFY_IMAGEGEN_URL", ""),
		CORSOrigins:   splitList(env("AURIFY_CORS_ORIGINS", "http://localhost:5173")),
		AppBaseURL:    env("AURIFY_APP_BASE_URL", "http://localhost:5173"),
		SupportEmail:  env("AURIFY_SUPPORT_EMAIL", "support@aurify.local"),
		Auth: AuthConfig{
			SigningKeySeed: env("AURIFY_AUTH_SIGNING_KEY", ""),
			Issuer:         env("AURIFY_AUTH_ISSUER", "aurify"),
			Audience:       env("AURIFY_AUTH_AUDIENCE", "aurify-api"),
			// Short, because the access token is verified statelessly: this is
			// how long a revoked session keeps working.
			AccessTTL: envDuration("AURIFY_AUTH_ACCESS_TTL", 15*time.Minute),
			// Idle timeout on the refresh token.
			RefreshTTL: envDuration("AURIFY_AUTH_REFRESH_TTL", 30*24*time.Hour),
			// Absolute cap on a session regardless of refreshes.
			SessionTTL:   envDuration("AURIFY_AUTH_SESSION_TTL", 90*24*time.Hour),
			CookieSecure: envBool("AURIFY_AUTH_COOKIE_SECURE", true),
		},
		SMTP: SMTPConfig{
			Host:     env("AURIFY_SMTP_HOST", ""),
			Port:     env("AURIFY_SMTP_PORT", "2525"),
			Username: env("AURIFY_SMTP_USERNAME", ""),
			Password: env("AURIFY_SMTP_PASSWORD", ""),
			From:     env("AURIFY_SMTP_FROM", "no-reply@aurify.local"),
		},
		DSP: DSPConfig{
			Spotify: OAuthConfig{
				ClientID:     env("AURIFY_SPOTIFY_CLIENT_ID", ""),
				ClientSecret: env("AURIFY_SPOTIFY_CLIENT_SECRET", ""),
				RedirectURL: env(
					"AURIFY_SPOTIFY_REDIRECT_URL",
					"http://localhost:8080/api/v1/auth/spotify/callback",
				),
			},
			AppleMusic: OAuthConfig{
				ClientID:     env("AURIFY_APPLE_CLIENT_ID", ""),
				ClientSecret: env("AURIFY_APPLE_CLIENT_SECRET", ""),
				RedirectURL: env(
					"AURIFY_APPLE_REDIRECT_URL",
					"http://localhost:8080/api/v1/auth/apple_music/callback",
				),
			},
			YouTubeMusic: OAuthConfig{
				ClientID:     env("AURIFY_YOUTUBE_CLIENT_ID", ""),
				ClientSecret: env("AURIFY_YOUTUBE_CLIENT_SECRET", ""),
				RedirectURL: env(
					"AURIFY_YOUTUBE_REDIRECT_URL",
					"http://localhost:8080/api/v1/auth/youtube_music/callback",
				),
			},
		},
	}
}

// envDuration reads a Go duration string (for example "15m"), falling back on
// an unparseable value rather than failing to boot over a typo.
func envDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

// envBool reads a boolean. An unparseable value keeps the fallback, which for
// security flags is the safe setting.
func envBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func splitList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
