// Package config loads runtime configuration from the environment. All
// settings have development-friendly defaults so the API can boot with zero
// configuration against a local MongoDB.
package config

import (
	"os"
	"strings"
)

// Config is the fully resolved application configuration.
type Config struct {
	HTTPAddr      string
	MongoURI      string
	MongoDatabase string
	LRCLibBaseURL string
	// PromptGenURL and ImageGenURL point at the local LLM sidecars. They are
	// intentionally empty by default; see docs/adr/0006-llm-sidecars.md.
	PromptGenURL string
	ImageGenURL  string
	CORSOrigins  []string
	DSP          DSPConfig
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
		HTTPAddr:      env("AURIFY_HTTP_ADDR", ":8080"),
		MongoURI:      env("AURIFY_MONGO_URI", "mongodb://localhost:27017"),
		MongoDatabase: env("AURIFY_MONGO_DB", "aurify"),
		LRCLibBaseURL: env("AURIFY_LRCLIB_URL", "https://lrclib.net"),
		PromptGenURL:  env("AURIFY_PROMPTGEN_URL", ""),
		ImageGenURL:   env("AURIFY_IMAGEGEN_URL", ""),
		CORSOrigins:   splitList(env("AURIFY_CORS_ORIGINS", "http://localhost:5173")),
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
