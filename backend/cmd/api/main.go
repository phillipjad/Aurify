// Command api is the Aurify HTTP API server. It wires the concrete adapters
// (PostgreSQL, DSP providers, lyrics/NLP, LLM sidecars) into the CQRS application
// layer and serves the mux router.
package main

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/analysis"
	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/command"
	"github.com/phillipjad/aurify/backend/internal/app/command/connectdsp"
	"github.com/phillipjad/aurify/backend/internal/app/command/deletecover"
	"github.com/phillipjad/aurify/backend/internal/app/command/generatecover"
	"github.com/phillipjad/aurify/backend/internal/app/command/refreshsession"
	"github.com/phillipjad/aurify/backend/internal/app/command/requestpasswordreset"
	"github.com/phillipjad/aurify/backend/internal/app/command/resetpassword"
	"github.com/phillipjad/aurify/backend/internal/app/command/signin"
	"github.com/phillipjad/aurify/backend/internal/app/command/signout"
	"github.com/phillipjad/aurify/backend/internal/app/command/signup"
	"github.com/phillipjad/aurify/backend/internal/app/command/verifyemail"
	"github.com/phillipjad/aurify/backend/internal/app/lockout"
	"github.com/phillipjad/aurify/backend/internal/app/ports"
	"github.com/phillipjad/aurify/backend/internal/app/query"
	"github.com/phillipjad/aurify/backend/internal/app/query/getcover"
	"github.com/phillipjad/aurify/backend/internal/app/query/getuser"
	"github.com/phillipjad/aurify/backend/internal/app/query/listcovers"
	"github.com/phillipjad/aurify/backend/internal/app/query/listplaylists"
	"github.com/phillipjad/aurify/backend/internal/app/sessions"
	"github.com/phillipjad/aurify/backend/internal/config"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp/applemusic"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp/spotify"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp/youtubemusic"
	"github.com/phillipjad/aurify/backend/internal/platform/email"
	"github.com/phillipjad/aurify/backend/internal/platform/llm/imagegen"
	"github.com/phillipjad/aurify/backend/internal/platform/llm/promptgen"
	"github.com/phillipjad/aurify/backend/internal/platform/lyrics/lrclib"
	"github.com/phillipjad/aurify/backend/internal/platform/nlp"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres"
	httptransport "github.com/phillipjad/aurify/backend/internal/transport/http"
	"github.com/phillipjad/aurify/backend/internal/transport/http/handlers"
)

// version is stamped at compile time via -ldflags="-X main.version=<semver>".
// The binary refuses to start if it is empty to prevent unversioned deployments.
var version string

func main() {
	if version == "" {
		slog.Error("build is missing version: recompile with -ldflags=\"-X main.version=<semver>\"")
		os.Exit(1)
	}
	if err := run(); err != nil {
		slog.Error("aurify api exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- storage ---
	store, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	// --- DSP providers + registry ---
	providers := dsp.NewRegistry(
		spotify.New(dsp.OAuthConfig(cfg.DSP.Spotify)),
		applemusic.New(dsp.OAuthConfig(cfg.DSP.AppleMusic)),
		youtubemusic.NewProvider(dsp.OAuthConfig(cfg.DSP.YouTubeMusic)),
	)

	// --- platform services ---
	lyricsClient := lrclib.New(cfg.LRCLibBaseURL, version)
	sentiment := nlp.NewAnalyzer()
	engine := analysis.NewEngine()
	prompts := promptgen.New(cfg.PromptGenURL)
	images := imagegen.New(cfg.ImageGenURL)

	// --- authentication ---
	signingKey, err := resolveSigningKey(cfg.Auth.SigningKeySeed)
	if err != nil {
		return err
	}
	signer, err := auth.NewSigner(signingKey, cfg.Auth.Issuer, cfg.Auth.Audience)
	if err != nil {
		return err
	}
	verifier, err := auth.NewVerifier(
		signingKey.Public().(ed25519.PublicKey),
		cfg.Auth.Issuer,
		cfg.Auth.Audience,
	)
	if err != nil {
		return err
	}
	issuer := sessions.NewIssuer(store.Sessions(), signer, sessions.TTL{
		Access:  cfg.Auth.AccessTTL,
		Refresh: cfg.Auth.RefreshTTL,
		Session: cfg.Auth.SessionTTL,
	})
	mailer := resolveMailer(cfg.SMTP)

	// --- application layer (lightweight CQRS) ---
	application := &app.App{
		Commands: &command.Bus{
			ConnectDSP: connectdsp.NewHandler(store.Users(), providers),
			GenerateCover: generatecover.NewHandler(
				store.Users(), store.Covers(), providers,
				lyricsClient, sentiment, engine, prompts, images,
			),
			DeleteCover: deletecover.NewHandler(store.Covers()),

			SignUp: signup.NewHandler(
				store.Users(), store.Credentials(), store.EmailTokens(), mailer, cfg.AppBaseURL,
			),
			SignIn:         signin.NewHandler(store.Users(), store.Credentials(), issuer),
			RefreshSession: refreshsession.NewHandler(issuer),
			SignOut:        signout.NewHandler(issuer),
			VerifyEmail:    verifyemail.NewHandler(store.EmailTokens(), store.Credentials()),
			RequestPasswordReset: requestpasswordreset.NewHandler(
				store.Users(), store.EmailTokens(), mailer, cfg.AppBaseURL,
			),
			ResetPassword: resetpassword.NewHandler(store.EmailTokens(), store.Credentials(), issuer),
		},
		Queries: &query.Bus{
			ListPlaylists: listplaylists.NewHandler(store.Users(), providers),
			GetCover:      getcover.NewHandler(store.Covers()),
			ListCovers:    listcovers.NewHandler(store.Covers()),
			GetUser:       getuser.NewHandler(store.Users()),
		},
	}

	// --- transport ---
	router, err := httptransport.NewRouter(httptransport.Deps{
		Application: application,
		Providers:   providers,
		Version:     version,
		CORSOrigins: cfg.CORSOrigins,
		Ready:       store.Ping,
		Verifier:    verifier,
		Cookies:     handlers.NewCookieWriter(cfg.Auth.CookieSecure),
		// Warn at five failures, permanently block the (IP, address) pair at
		// ten within fifteen minutes. This is the control that keeps signup's
		// "already registered" answer from scaling into bulk enumeration, and
		// that makes credential stuffing against sign-in expensive.
		Guard:        lockout.NewGuard(store.AuthBlocks()),
		SupportEmail: cfg.SupportEmail,
		// Makes sign-out and refresh-reuse revocation take effect at once
		// instead of lagging by the access-token lifetime.
		SessionCheck: issuer.Verify,
	})
	if err != nil {
		return err
	}

	slog.Info("starting aurify api", "addr", cfg.HTTPAddr)
	server := mux.NewServer(cfg.HTTPAddr, router)
	return server.Listen(ctx)
}

// resolveSigningKey loads the configured Ed25519 seed, or generates an
// ephemeral key when none is set.
//
// The ephemeral path is a development convenience and is logged loudly: keys
// that change on restart invalidate every outstanding access token, and a
// second instance would sign with a key the first cannot verify. Production
// must set AURIFY_AUTH_SIGNING_KEY.
func resolveSigningKey(seed string) (ed25519.PrivateKey, error) {
	if seed != "" {
		return auth.ParsePrivateKeySeed(seed)
	}

	generated, err := auth.GenerateKeySeed()
	if err != nil {
		return nil, err
	}
	slog.Warn("AURIFY_AUTH_SIGNING_KEY is not set, generating an ephemeral signing key; " +
		"sessions will not survive a restart and multiple instances will reject each other's tokens")
	return auth.ParsePrivateKeySeed(generated)
}

// resolveMailer picks the SMTP relay when one is configured, and otherwise the
// development sender that logs messages instead of delivering them.
func resolveMailer(cfg config.SMTPConfig) ports.EmailSender {
	if cfg.Host == "" {
		slog.Warn("AURIFY_SMTP_HOST is not set, verification and reset emails will be written to the log")
		return email.LogSender{}
	}
	sender, err := email.NewSMTPSender(email.SMTPConfig{
		Host:     cfg.Host,
		Port:     cfg.Port,
		Username: cfg.Username,
		Password: cfg.Password,
		From:     cfg.From,
	})
	if err != nil {
		slog.Error("smtp configuration is invalid, falling back to the log sender", "error", err)
		return email.LogSender{}
	}
	return sender
}
