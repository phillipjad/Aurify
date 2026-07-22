// Command api is the Aurify HTTP API server. It wires the concrete adapters
// (PostgreSQL, DSP providers, lyrics/NLP, LLM sidecars) into the CQRS application
// layer and serves the mux router.
package main

import (
	"context"
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
	"github.com/phillipjad/aurify/backend/internal/app/query"
	"github.com/phillipjad/aurify/backend/internal/app/query/getcover"
	"github.com/phillipjad/aurify/backend/internal/app/query/listcovers"
	"github.com/phillipjad/aurify/backend/internal/app/query/listplaylists"
	"github.com/phillipjad/aurify/backend/internal/config"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp/applemusic"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp/spotify"
	"github.com/phillipjad/aurify/backend/internal/platform/dsp/youtubemusic"
	"github.com/phillipjad/aurify/backend/internal/platform/llm/imagegen"
	"github.com/phillipjad/aurify/backend/internal/platform/llm/promptgen"
	"github.com/phillipjad/aurify/backend/internal/platform/lyrics/lrclib"
	"github.com/phillipjad/aurify/backend/internal/platform/nlp"
	"github.com/phillipjad/aurify/backend/internal/storage/postgres"
	httptransport "github.com/phillipjad/aurify/backend/internal/transport/http"
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

	// --- application layer (lightweight CQRS) ---
	application := &app.App{
		Commands: &command.Bus{
			ConnectDSP: connectdsp.NewHandler(store.Users(), providers),
			GenerateCover: generatecover.NewHandler(
				store.Users(), store.Covers(), providers,
				lyricsClient, sentiment, engine, prompts, images,
			),
			DeleteCover: deletecover.NewHandler(store.Covers()),
		},
		Queries: &query.Bus{
			ListPlaylists: listplaylists.NewHandler(store.Users(), providers),
			GetCover:      getcover.NewHandler(store.Covers()),
			ListCovers:    listcovers.NewHandler(store.Covers()),
		},
	}

	// --- transport ---
	router, err := httptransport.NewRouter(application, providers, version, cfg.CORSOrigins, store.Ping)
	if err != nil {
		return err
	}

	slog.Info("starting aurify api", "addr", cfg.HTTPAddr)
	server := mux.NewServer(cfg.HTTPAddr, router)
	return server.Listen(ctx)
}
