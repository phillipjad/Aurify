// Command openapi generates the Aurify OpenAPI specification from the live mux
// routes and writes it to a file. The spec is the single source of truth for
// the API contract: the frontend generates its TypeScript types from it, so the
// wire format cannot drift from the Go handlers and DTOs.
//
// Regenerate with `go generate ./...` (from the backend module root), or run it
// directly:
//
//	go run ./cmd/openapi -out api/openapi.yaml
package main

//go:generate go run . -out ../../api/openapi.yaml

import (
	"crypto/ed25519"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fgrzl/mux"

	"github.com/phillipjad/aurify/backend/internal/platform/auth"
	"github.com/phillipjad/aurify/backend/internal/transport/http/handlers"

	"github.com/phillipjad/aurify/backend/internal/app"
	httptransport "github.com/phillipjad/aurify/backend/internal/transport/http"
)

func main() {
	out := flag.String("out", "api/openapi.yaml", "output path for the spec (.yaml or .json)")
	version := flag.String("version", "dev", "API version written to the spec info object")
	flag.Parse()

	if err := generate(*out, *version); err != nil {
		slog.Error("openapi generation failed", "error", err)
		os.Exit(1)
	}
	slog.Info("wrote OpenAPI spec", "path", *out)
}

func generate(out, version string) error {
	// Build the router with no live dependencies: route registration only stores
	// handler values and OpenAPI metadata, so this never touches PostgreSQL, a DSP,
	// or the LLM sidecars.
	//
	// The auth collaborators are constructed because the middleware closures
	// capture them, but they are never invoked: no request is served here. The
	// signing key is throwaway for the same reason.
	seed, err := auth.GenerateKeySeed()
	if err != nil {
		return err
	}
	key, err := auth.ParsePrivateKeySeed(seed)
	if err != nil {
		return err
	}
	verifier, err := auth.NewVerifier(key.Public().(ed25519.PublicKey), "aurify", "aurify-api")
	if err != nil {
		return err
	}

	router, err := httptransport.NewRouter(httptransport.Deps{
		Application: &app.App{},
		Version:     version,
		Verifier:    verifier,
		Cookies:     handlers.NewCookieWriter(true),
		Throttle:    handlers.NewThrottle(10, 15*time.Minute),
	})
	if err != nil {
		return err
	}

	spec, err := mux.GenerateSpecWithGenerator(mux.NewGenerator(mux.WithOpenAPIExamples()), router)
	if err != nil {
		return err
	}

	if dir := filepath.Dir(out); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return spec.Normalize().MarshalToFile(out)
}
