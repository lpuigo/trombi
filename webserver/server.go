// Package webserver is the web GUI's handler layer (Docs/SPEC_trombinoscope.md
// §10.2): it serves the embedded single-page UI and a JSON API over a
// service.Workspace. Like the CLI, it only parses requests, calls the
// service layer and serializes responses — no detection or framing logic
// lives here.
package webserver

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"trombi/service"
)

//go:embed static
var staticFiles embed.FS

// Server holds the Workspace the API operates on.
type Server struct {
	ws  *service.Workspace
	log *slog.Logger
}

// New returns the GUI's HTTP handler, restricted to local requests (see
// localOnly).
func New(ws *service.Workspace, log *slog.Logger) http.Handler {
	s := &Server{ws: ws, log: log}

	static, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err) // the embedded directory is fixed at build time
	}

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("GET /api/state", s.getState)
	mux.HandleFunc("POST /api/items", s.addItem)
	mux.HandleFunc("PATCH /api/items/{id}", s.patchItem)
	mux.HandleFunc("DELETE /api/items/{id}", s.deleteItem)
	mux.HandleFunc("POST /api/items/{id}/reset", s.resetItem)
	mux.HandleFunc("GET /api/items/{id}/portrait.jpg", s.getPortrait)
	mux.HandleFunc("GET /api/items/{id}/source.jpg", s.getSource)
	mux.HandleFunc("PUT /api/settings/framing", s.putFraming)
	mux.HandleFunc("PUT /api/settings/layout", s.putLayout)
	mux.HandleFunc("GET /api/export.pdf", s.exportPDF)
	return localOnly(mux)
}
