package http

import (
	"embed"
	"io/fs"
	"net/http"
)

// uiFiles holds the compiled React SPA. In Docker builds the frontend dist is
// copied here before the Go binary is compiled; locally, the directory contains
// only a .gitkeep so the embed succeeds while developers use `pnpm dev`.
//
//go:embed all:ui
var uiFiles embed.FS

// newSPAHandler returns an http.Handler that serves the embedded SPA and falls
// back to index.html for any path that does not match a real asset, enabling
// client-side routing.
func newSPAHandler() http.Handler {
	sub, _ := fs.Sub(uiFiles, "ui")
	return http.FileServer(spaFS{http.FS(sub)})
}

// spaFS wraps http.FileSystem so that any missing file is served as index.html
// instead of a 404, which is required for React Router's client-side navigation.
type spaFS struct{ http.FileSystem }

func (s spaFS) Open(name string) (http.File, error) {
	f, err := s.FileSystem.Open(name)
	if err != nil && name != "/index.html" {
		return s.FileSystem.Open("/index.html")
	}
	return f, err
}
