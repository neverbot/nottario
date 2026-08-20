package web

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// Static assets are immutable for the lifetime of the process, so they
// are compressed once at startup rather than on every request. That
// costs a few hundred KB of resident memory and zero per-request CPU,
// which is the right trade when the same bytes would otherwise be
// recompressed for every visitor.
//
// gzip only. Brotli would buy a further 15-20%, but it is not in the
// standard library, and "every dependency justifies its presence" is a
// project invariant that one more compression codec does not clear.

// compressibleExts are the types worth gzipping. Anything already
// compressed (PNG, JPEG, WOFF2) is deliberately absent: re-compressing
// it burns CPU and can make the payload larger.
var compressibleExts = map[string]bool{
	".js":   true,
	".mjs":  true,
	".css":  true,
	".html": true,
	".json": true,
	".svg":  true,
	".txt":  true,
	".md":   true,
	".map":  true,
	".xml":  true,
}

// gzipAsset is one precompressed representation. It carries its own
// ETag: the gzipped bytes are a different representation of the
// resource than the identity bytes, so they must not share a
// validator, or a cache keyed on ETag could hand the wrong encoding to
// a client.
type gzipAsset struct {
	body []byte
	etag string
}

var (
	staticGzipOnce sync.Once
	staticGzip     map[string]gzipAsset
)

// gzipAssetFor returns the precompressed variant for a path relative
// to the static root, if one exists.
func gzipAssetFor(rel string) (gzipAsset, bool) {
	staticGzipOnce.Do(buildStaticGzip)
	a, ok := staticGzip[rel]
	return a, ok
}

func buildStaticGzip() {
	staticGzip = make(map[string]gzipAsset)
	_ = fs.WalkDir(staticFS, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !compressibleExts[strings.ToLower(path.Ext(p))] {
			return err
		}
		data, readErr := fs.ReadFile(staticFS, p)
		if readErr != nil {
			return nil
		}
		encoded, ok := gzipBytes(data)
		if !ok {
			return nil
		}
		staticGzip[strings.TrimPrefix(p, "static/")] = encoded
		return nil
	})
}

// gzipBytes compresses content, reporting false when the result is not
// actually smaller. Tiny files can grow once the gzip header and
// trailer are added, and shipping those compressed would be a pure
// loss.
func gzipBytes(content []byte) (gzipAsset, bool) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return gzipAsset{}, false
	}
	if _, err := zw.Write(content); err != nil {
		_ = zw.Close()
		return gzipAsset{}, false
	}
	if err := zw.Close(); err != nil {
		return gzipAsset{}, false
	}
	if buf.Len() >= len(content) {
		return gzipAsset{}, false
	}
	body := buf.Bytes()
	return gzipAsset{body: body, etag: etagOf(body)}, true
}

// acceptsGzip reports whether the client advertised gzip support.
func acceptsGzip(r *http.Request) bool {
	for _, enc := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		// Strip any q-value; "gzip;q=0" is a refusal, not an offer.
		name, params, _ := strings.Cut(strings.TrimSpace(enc), ";")
		if strings.EqualFold(name, "gzip") {
			return !strings.Contains(strings.ReplaceAll(params, " ", ""), "q=0")
		}
	}
	return false
}

// contentTypeFor resolves the media type from the file extension. We
// set it explicitly whenever we serve compressed bytes, because
// http.ServeContent would otherwise sniff the gzip stream and label
// everything as an archive.
func contentTypeFor(rel string) string {
	if ct := mime.TypeByExtension(path.Ext(rel)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// withStaticAssets serves the precompressed variant to clients that
// accept gzip and falls through to the file server otherwise, tagging
// both paths with the appropriate validator.
//
// Vary is set on every response, compressed or not: without it a
// shared cache could store the gzipped bytes and later hand them to a
// client that never asked for them.
func withStaticAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(path.Clean(r.URL.Path), "/static/")
		w.Header().Set("Vary", "Accept-Encoding")

		if asset, ok := gzipAssetFor(rel); ok && acceptsGzip(r) {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Type", contentTypeFor(rel))
			w.Header().Set("ETag", asset.etag)
			// Empty name so ServeContent does not re-derive the type
			// from an extension; we have already set it.
			http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(asset.body))
			return
		}

		if tag := staticETagFor(rel); tag != "" {
			w.Header().Set("ETag", tag)
		}
		next.ServeHTTP(w, r)
	})
}
