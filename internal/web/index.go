package web

import (
	"bytes"
	"io/fs"
	"net/http"
	"strings"
	"time"
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
//
// The shell used to carry a ?v=<hash> query on its script and
// stylesheet URLs to defeat stale caches. That only ever covered the
// two entry points — the inner ES module imports were never hashed —
// and it became redundant once every static response started carrying
// an ETag (see etag.go), which invalidates entry points and modules
// alike through the same mechanism.
func IndexHandler() http.Handler {
	html, err := fs.ReadFile(staticFS, "static/index.html")
	if err != nil {
		panic("nottario: missing embedded static/index.html: " + err.Error())
	}
	// The shell is rendered once and never changes while the process
	// runs, so it gets the same treatment as the assets it references:
	// a strong validator, so a navigation costs a 304 instead of
	// re-sending the document. http.ServeContent handles the
	// conditional request against the ETag we set here.
	etag := etagOf(html)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, p := range reservedPrefixes {
			if strings.HasPrefix(r.URL.Path, p) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		w.Header().Set("ETag", etag)
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(html))
	})
}
