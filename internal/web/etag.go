package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
)

// Static assets ship inside the binary, so their bytes cannot change
// while the process runs. That makes them ideal ETag candidates: hash
// every file once at startup and serve the digest as a strong
// validator forever after.
//
// This is the missing half of the caching bargain. The static handler
// sends `Cache-Control: no-cache, must-revalidate`, which is correct —
// it is what stops a stale bundle surviving a rebuild — but embed.FS
// reports a zero modtime, so http.ServeContent omits Last-Modified and
// http.FileServer generates no ETag. With no validator at all the
// browser cannot make a conditional request, so "revalidate" degraded
// into "download the whole thing again", every single load.
//
// Setting the header before delegating is enough to fix that:
// net/http's serveContent calls checkPreconditions, which reads
// w.Header().Get("Etag") and answers a matching If-None-Match with a
// 304 on our behalf.

var (
	staticETagsOnce sync.Once
	staticETags     map[string]string
)

// staticETagFor returns the quoted strong ETag for a path relative to
// the static root (e.g. "app.js", "components/toast.js"), or "" when
// the path is not an embedded asset.
func staticETagFor(rel string) string {
	staticETagsOnce.Do(buildStaticETags)
	return staticETags[rel]
}

// buildStaticETags walks the embedded tree once and hashes each file.
// Reading the whole tree costs a few megabytes of transient work at
// startup and nothing per request.
func buildStaticETags() {
	staticETags = make(map[string]string)
	_ = fs.WalkDir(staticFS, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, readErr := fs.ReadFile(staticFS, p)
		if readErr != nil {
			return nil // unreadable asset simply gets no validator
		}
		rel := strings.TrimPrefix(p, "static/")
		staticETags[rel] = etagOf(data)
		return nil
	})
}

// etagOf renders content as a quoted strong ETag. Truncated to 16 hex
// characters: still 64 bits of collision resistance, and the header
// stays short enough to read in a network panel.
func etagOf(content []byte) string {
	sum := sha256.Sum256(content)
	return `"` + hex.EncodeToString(sum[:])[:16] + `"`
}

// withStaticETag wraps the static file server, attaching the
// precomputed validator for the requested asset. Requests for unknown
// paths fall through untouched so the file server can 404 them.
func withStaticETag(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(path.Clean(r.URL.Path), "/static/")
		if tag := staticETagFor(rel); tag != "" {
			w.Header().Set("ETag", tag)
		}
		next.ServeHTTP(w, r)
	})
}
