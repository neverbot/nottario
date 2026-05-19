package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"html/template"
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
//
// The HTML is templated with a hash of every embedded static file,
// so the referenced JS/CSS URLs change on every rebuild (?v=<hash>),
// sidestepping any browser cache for stale entry assets. Inner ES
// module imports still rely on Cache-Control headers, but the
// browser will at least always pick up the new app.js entry point
// from which the rest is re-resolved.
func IndexHandler() http.Handler {
	raw, err := fs.ReadFile(staticFS, "static/index.html")
	if err != nil {
		panic("nottario: missing embedded static/index.html: " + err.Error())
	}
	tpl, err := template.New("index").Parse(string(raw))
	if err != nil {
		panic("nottario: index.html template parse: " + err.Error())
	}
	var rendered bytes.Buffer
	if err := tpl.Execute(&rendered, map[string]string{"Version": staticAssetsHash()}); err != nil {
		panic("nottario: index.html render: " + err.Error())
	}
	html := rendered.Bytes()
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

// staticAssetsHash computes a short hash from every file in the
// embedded static tree. Used as a cache-busting URL parameter on the
// HTML's <script> and <link> entries.
func staticAssetsHash() string {
	h := sha256.New()
	_ = fs.WalkDir(staticFS, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, _ := fs.ReadFile(staticFS, p)
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(data)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))[:12]
}
