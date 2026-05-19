package web

import (
	"io/fs"
	"net/http"
	"strings"
)

// reservedPrefixes are URL prefixes that must never fall through to
// the SPA shell. A request to an unmatched /api/... or /auth/... is a
// client mistake and should get a 404 — not an HTML page.
var reservedPrefixes = []string{
	"/api/",
	"/auth/",
	"/mcp",
	"/skill",
	"/static/",
	"/healthz",
	"/version",
	"/events",
}

// IndexHandler serves the embedded index.html for any path the
// client-side router owns. It works as a catch-all: any unknown path
// receives the SPA shell so the in-page router can resolve it.
// Paths under reservedPrefixes return 404 instead.
func IndexHandler() http.Handler {
	html, err := fs.ReadFile(staticFS, "static/index.html")
	if err != nil {
		panic("nottario: missing embedded static/index.html: " + err.Error())
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, p := range reservedPrefixes {
			if strings.HasPrefix(r.URL.Path, p) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(html)
	})
}
