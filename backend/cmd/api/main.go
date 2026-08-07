// Command api is the Aurify HTTP API server. It wires the concrete adapters
// (PostgreSQL, DSP providers, lyrics/NLP, prompt + image models) into the CQRS application
// layer and serves the mux router.
package main

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/analysis"
	"github.com/phillipjad/aurify/backend/internal/app"
	"github.com/phillipjad/aurify/backend/internal/app/command"
	"github.com/phillipjad/aurify/backend/internal/app/command/connectdsp"
	"github.com/phillipjad/aurify/backend/internal/app/command/deletecover"
	"github.com/phillipjad/aurify/backend/internal/app/command/federatedsignin"
	"github.com/phillipjad/aurify/backend/internal/app/command/generatecover"
	"github.com/phillipjad/aurify/backend/internal/app/command/refreshsession"
	"github.com/phillipjad/aurify/backend/internal/app/command/requestpasswordreset"
	"github.com/phillipjad/aurify/backend/internal/app/command/resetpassword"
	"github.com/phillipjad/aurify/backend/internal/app/command/signin"
	"github.com/phillipjad/aurify/backend/internal/app/command/signout"
	"github.com/phillipjad/aurify/backend/internal/app/command/signup"
	"github.com/phillipjad/aurify/backend/internal/app/command/verifyemail"
	"github.com/phillipjad/aurify/backend/internal/app/dspconn"
	"github.com/phillipjad/aurify/backend/internal/app/lockout"
	"github.com/phillipjad/aurify/backend/internal/app/lyrics"
	"github.com/phillipjad/aurify/backend/internal/app/query"
	"github.com/phillipjad/aurify/backend/internal/app/query/getcover"
	"github.com/phillipjad/aurify/backend/internal/app/query/getcoverimage"
	"github.com/phillipjad/aurify/backend/internal/app/query/getuser"
	"github.com/phillipjad/aurify/backend/internal/app/query/listcovers"
	"github.com/phillipjad/aurify/backend/internal/app/query/listplaylists"
	"github.com/phillipjad/aurify/backend/internal/app/sessions"
	"github.com/phillipjad/aurify/backend/internal/config"
	"github.com/phillipjad/aurify/backend/internal/platform/auth"
	"github.com/phillipjad/aurify/backend/internal/platform/crypto"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp/applemusic"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp/spotify"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp/youtubemusic"
	"github.com/phillipjad/aurify/backend/internal/platform/email"
	"github.com/phillipjad/aurify/backend/internal/platform/features/acousticbrainz"
	"github.com/phillipjad/aurify/backend/internal/platform/identity/google"
	llmfeatures "github.com/phillipjad/aurify/backend/internal/platform/llm/features"
	"github.com/phillipjad/aurify/backend/internal/platform/llm/imagegen"
	"github.com/phillipjad/aurify/backend/internal/platform/llm/promptgen"
	"github.com/phillipjad/aurify/backend/internal/platform/lyrics/breaker"
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
	// Fail fast, before anything is constructed. A deployment missing a mail
	// relay used to start happily and then write live reset tokens into the log.
	if err := cfg.Validate(); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The signing seed is resolved first: the key that encrypts stored DSP tokens
	// is derived from it, so storage cannot be constructed before it exists.
	signingKey, err := resolveSigningKey(cfg.Auth.SigningKeySeed)
	if err != nil {
		return err
	}

	// --- storage ---
	store, err := postgres.Connect(ctx, cfg.DatabaseURL, crypto.DeriveKey(signingKey.Seed()))
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
	// Composed innermost first: the network client, a circuit breaker so a
	// failing provider is left alone, and a resolver that batches lookups through
	// the caches. The caches sit outside the breaker on purpose, so a cached
	// answer is still served while the circuit is open.
	lyricsResolver := lyrics.NewResolver(
		store.Lyrics(),
		breaker.New(lrclib.New(cfg.LRCLibBaseURL, version)),
	)
	sentiment := nlp.NewAnalyzer()
	engine := analysis.NewEngine()
	prompts := promptgen.New(cfg.PromptGen.BaseURL, cfg.PromptGen.Model, cfg.PromptGen.APIKey)
	// Acoustic features for tracks the DSP left empty. AcousticBrainz first,
	// since it is measured; the text model only when nothing matched at all.
	trackFeatures := acousticbrainz.New(store.TrackFeatures(), version)
	featureEstimator := llmfeatures.New(
		cfg.PromptGen.BaseURL, cfg.PromptGen.Model, cfg.PromptGen.APIKey,
	)
	images := imagegen.New(
		cfg.ImageGen.BaseURL,
		cfg.ImageGen.AccountID,
		cfg.ImageGen.Model,
		cfg.ImageGen.APIKey,
	)

	// --- authentication ---
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
	mailer, err := email.Resolve(email.SMTPConfig{
		Host:     cfg.SMTP.Host,
		Port:     cfg.SMTP.Port,
		Username: cfg.SMTP.Username,
		Password: cfg.SMTP.Password,
		From:     cfg.SMTP.From,
		TLS:      cfg.SMTP.TLS,
	})
	if err != nil {
		return err
	}

	// Sign in with Google. Empty credentials are not an error: the provider then
	// reports itself disabled and its routes answer 501, so a deployment that
	// does not want federated sign-in simply leaves the variables unset.
	googleProvider, err := google.NewProvider(google.Config{
		ClientID:     cfg.Google.ClientID,
		ClientSecret: cfg.Google.ClientSecret,
		RedirectURL:  cfg.Google.RedirectURL,
	})
	if err != nil {
		return err
	}
	if !googleProvider.Enabled() {
		slog.Warn("AURIFY_GOOGLE_CLIENT_ID/SECRET are not set, Sign in with Google is disabled")
	}

	// Resolves a user's DSP credentials, refreshing and storing them when they
	// have expired, so both the read and write side see a current token.
	connections := dspconn.NewResolver(store.Users(), providers)

	// --- application layer (lightweight CQRS) ---
	application := &app.App{
		Commands: &command.Bus{
			ConnectDSP: connectdsp.NewHandler(store.Users(), providers),
			GenerateCover: generatecover.NewHandler(
				connections, store.Covers(),
				lyricsResolver, sentiment, engine, prompts, images, store.CoverImages(),
				trackFeatures, featureEstimator,
			),
			DeleteCover: deletecover.NewHandler(store.Covers()),

			SignUp: signup.NewHandler(
				store.Users(), store.Credentials(), store.EmailTokens(), mailer, cfg.AppBaseURL,
			),
			SignIn: signin.NewHandler(store.Users(), store.Credentials(), issuer),
			FederatedSignIn: federatedsignin.NewHandler(
				store.Users(), store.Identities(), issuer,
			),
			RefreshSession: refreshsession.NewHandler(issuer),
			SignOut:        signout.NewHandler(issuer),
			VerifyEmail:    verifyemail.NewHandler(store.EmailTokens(), store.Credentials()),
			RequestPasswordReset: requestpasswordreset.NewHandler(
				store.Users(), store.EmailTokens(), mailer, cfg.AppBaseURL,
			),
			ResetPassword: resetpassword.NewHandler(store.EmailTokens(), store.Credentials(), issuer),
		},
		Queries: &query.Bus{
			ListPlaylists: listplaylists.NewHandler(connections),
			GetCover:      getcover.NewHandler(store.Covers()),
			GetCoverImage: getcoverimage.NewHandler(store.CoverImages()),
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
		Google:       googleProvider,
		AppBaseURL:   cfg.AppBaseURL,
		// Derived from the authentication seed rather than configured separately,
		// so there is no second secret to deploy.
		FlowKey: handlers.DeriveFlowKey(signingKey.Seed()),
		// Push for the SSE streams: a trigger NOTIFYs, this LISTENs (ADR 0019).
		WatchCover: store.WatchCovers(ctx).Subscribe,
	})
	if err != nil {
		return err
	}

	// Generation runs in-process (ADR 0019), so a crash orphans in-flight covers
	// in a non-terminal status. The cutoff is far beyond the longest real run, so
	// another instance's active pipeline is never swept.
	go func() {
		const staleAfter = 10 * time.Minute
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			if n, err := store.Covers().FailStuck(ctx, time.Now().UTC().Add(-staleAfter)); err != nil {
				slog.Warn("sweeping stuck covers failed", "error", err)
			} else if n > 0 {
				slog.Info("failed stuck covers", "count", n)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	slog.Info("starting aurify api", "addr", cfg.HTTPAddr)
	server := mux.NewServer(cfg.HTTPAddr, router)
	return server.Listen(ctx)
}

// resolveSigningKey loads the configured Ed25519 seed.
//
// There is no generated fallback. A key that changes on restart silently signs
// every user out, and two instances holding different keys reject each other's
// tokens, which surfaces as intermittent 401s that are painful to trace back to
// a missing variable. Config.Validate rejects an empty seed before we get here.
func resolveSigningKey(seed string) (ed25519.PrivateKey, error) {
	return auth.ParsePrivateKeySeed(seed)
}
