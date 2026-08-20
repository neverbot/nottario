package web

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serveStaticEnc drives a request through the real server wiring with
// an explicit Accept-Encoding.
func serveStaticEnc(t *testing.T, path, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewServer(Deps{})
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestGzipServedWhenAccepted(t *testing.T) {
	plain := serveStaticEnc(t, "/static/app.js", "")
	zipped := serveStaticEnc(t, "/static/app.js", "gzip")

	if enc := zipped.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", enc)
	}
	if zipped.Body.Len() >= plain.Body.Len() {
		t.Errorf("gzip body (%d) not smaller than identity (%d)", zipped.Body.Len(), plain.Body.Len())
	}

	// The compressed bytes must decode back to exactly what the
	// identity response would have sent.
	zr, err := gzip.NewReader(strings.NewReader(zipped.Body.String()))
	if err != nil {
		t.Fatalf("response is not valid gzip: %v", err)
	}
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	if string(decoded) != plain.Body.String() {
		t.Error("decompressed body differs from the identity body")
	}
}

func TestIdentityServedWhenGzipNotAccepted(t *testing.T) {
	rr := serveStaticEnc(t, "/static/app.js", "")
	if enc := rr.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("Content-Encoding = %q, want none for a client that did not ask", enc)
	}
	if !strings.Contains(rr.Body.String(), "import") {
		t.Error("identity response does not look like the JS source")
	}
}

func TestGzipRefusedWithQZero(t *testing.T) {
	// "gzip;q=0" is an explicit refusal, not an offer.
	rr := serveStaticEnc(t, "/static/app.js", "gzip;q=0")
	if enc := rr.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("Content-Encoding = %q, want none when gzip is refused", enc)
	}
}

func TestVaryAlwaysSet(t *testing.T) {
	for _, ae := range []string{"", "gzip"} {
		rr := serveStaticEnc(t, "/static/app.js", ae)
		if v := rr.Header().Get("Vary"); !strings.Contains(v, "Accept-Encoding") {
			t.Errorf("Accept-Encoding %q: Vary = %q, want it to include Accept-Encoding", ae, v)
		}
	}
}

func TestGzipAndIdentityHaveDistinctETags(t *testing.T) {
	// Two encodings are two representations. Sharing a validator would
	// let a shared cache serve the wrong one.
	plain := serveStaticEnc(t, "/static/app.js", "").Header().Get("ETag")
	zipped := serveStaticEnc(t, "/static/app.js", "gzip").Header().Get("ETag")
	if plain == "" || zipped == "" {
		t.Fatalf("missing ETag: identity=%q gzip=%q", plain, zipped)
	}
	if plain == zipped {
		t.Errorf("identity and gzip share the ETag %s", plain)
	}
}

func TestGzipRevalidatesTo304(t *testing.T) {
	tag := serveStaticEnc(t, "/static/app.js", "gzip").Header().Get("ETag")

	h := NewServer(Deps{})
	req := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("If-None-Match", tag)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("304 carried a %d-byte body", rr.Body.Len())
	}
}

func TestGzipKeepsContentType(t *testing.T) {
	// Serving the gzip stream bypasses the file server, so the media
	// type has to be set explicitly — otherwise ServeContent sniffs the
	// compressed bytes and calls everything an archive.
	rr := serveStaticEnc(t, "/static/app.js", "gzip")
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q, want a JavaScript type", ct)
	}
	css := serveStaticEnc(t, "/static/styles.css", "gzip")
	if ct := css.Header().Get("Content-Type"); !strings.Contains(ct, "css") {
		t.Errorf("Content-Type = %q, want a CSS type", ct)
	}
}

func TestAlreadyCompressedTypesAreNotGzipped(t *testing.T) {
	// Nothing in compressibleExts covers images; re-compressing them
	// burns CPU for no gain.
	if _, ok := gzipAssetFor("screenshots/kanban.png"); ok {
		t.Error("a PNG was precompressed")
	}
	if compressibleExts[".png"] || compressibleExts[".woff2"] {
		t.Error("an already-compressed type is listed as compressible")
	}
}

func TestIndexShellIsGzipped(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	IndexHandler().ServeHTTP(rr, req)

	if enc := rr.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip for the SPA shell", enc)
	}
	if v := rr.Header().Get("Vary"); !strings.Contains(v, "Accept-Encoding") {
		t.Errorf("Vary = %q on the shell", v)
	}
	zr, err := gzip.NewReader(strings.NewReader(rr.Body.String()))
	if err != nil {
		t.Fatalf("shell is not valid gzip: %v", err)
	}
	decoded, _ := io.ReadAll(zr)
	if !strings.Contains(string(decoded), "<nottario-shell></nottario-shell>") {
		t.Error("decompressed shell does not contain the root element")
	}
}

func TestGzipSkippedWhenItWouldGrow(t *testing.T) {
	// A few bytes of incompressible input costs more than it saves once
	// the gzip header and trailer are added.
	if _, ok := gzipBytes([]byte("x")); ok {
		t.Error("compressed a payload that gzip makes larger")
	}
}
