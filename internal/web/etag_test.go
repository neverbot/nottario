package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// serveStatic drives a request through the same wrapping the real
// server applies to /static/, so the tests exercise the wiring rather
// than the middleware in isolation.
func serveStatic(t *testing.T, path, ifNoneMatch string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewServer(Deps{})
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestStaticAssetCarriesETag(t *testing.T) {
	rr := serveStatic(t, "/static/app.js", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if rr.Header().Get("ETag") == "" {
		t.Error("no ETag on a static asset: revalidation cannot produce a 304")
	}
	if rr.Body.Len() == 0 {
		t.Error("empty body on a fresh request")
	}
}

func TestStaticAssetRevalidatesTo304(t *testing.T) {
	first := serveStatic(t, "/static/app.js", "")
	tag := first.Header().Get("ETag")
	if tag == "" {
		t.Fatal("first response carried no ETag")
	}

	second := serveStatic(t, "/static/app.js", tag)
	if second.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304 when If-None-Match matches", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Errorf("304 carried a %d-byte body, want empty", second.Body.Len())
	}
}

func TestStaticAssetStaleETagGetsFullBody(t *testing.T) {
	// The regression this whole mechanism must not introduce: a client
	// holding an outdated validator has to receive the new bytes.
	rr := serveStatic(t, "/static/app.js", `"0000000000000000"`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a stale validator", rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Error("stale validator got an empty body")
	}
}

func TestStaticETagsDifferPerAsset(t *testing.T) {
	js := serveStatic(t, "/static/app.js", "").Header().Get("ETag")
	css := serveStatic(t, "/static/styles.css", "").Header().Get("ETag")
	if js == "" || css == "" {
		t.Fatalf("missing ETag: app.js=%q styles.css=%q", js, css)
	}
	if js == css {
		t.Errorf("distinct assets share an ETag (%s): a changed file would not invalidate", js)
	}
}

func TestStaticCacheControlUnchanged(t *testing.T) {
	// ETags supplement no-cache, they do not replace it.
	rr := serveStatic(t, "/static/app.js", "")
	if got := rr.Header().Get("Cache-Control"); got != "no-cache, must-revalidate" {
		t.Errorf("Cache-Control = %q, want it left alone", got)
	}
}

func TestIndexShellRevalidatesTo304(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	IndexHandler().ServeHTTP(rr, req)
	tag := rr.Header().Get("ETag")
	if tag == "" {
		t.Fatal("SPA shell carried no ETag")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("If-None-Match", tag)
	rr2 := httptest.NewRecorder()
	IndexHandler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304 for the shell", rr2.Code)
	}
}

func TestUnknownStaticPathStillMissing(t *testing.T) {
	rr := serveStatic(t, "/static/does-not-exist.js", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
	if rr.Header().Get("ETag") != "" {
		t.Error("a missing asset was given an ETag")
	}
}
