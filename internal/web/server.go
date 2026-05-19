package web

import (
	"io/fs"
	"net/http"
)

// NewServer returns an http.Handler wiring all M0 routes.
func NewServer() http.Handler {
	mux := http.NewServeMux()

	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("nottario: cannot derive static sub-fs: " + err.Error())
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	mux.Handle("GET /healthz", HealthzHandler())
	mux.Handle("GET /version", VersionHandler())
	mux.Handle("GET /{$}", IndexHandler())

	return mux
}
