package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewServer_PublicRoutes verifies the routes that don't require
// a database. Full identity-dependent routes are covered by the
// end-to-end smoke (compose stack + manual run).
func TestNewServer_PublicRoutes(t *testing.T) {
	srv := NewServer(Deps{})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	cases := []struct {
		path        string
		wantStatus  int
		wantSubstr  string
		wantCTPrefx string
	}{
		{"/", 200, "<nottario-shell></nottario-shell>", "text/html"},
		{"/healthz", 200, `"status":"ok"`, "application/json"},
		{"/version", 200, `"version"`, "application/json"},
		{"/static/styles.css", 200, "--fg:", "text/css"},
	}
	for _, c := range cases {
		resp, err := http.Get(ts.URL + c.path)
		if err != nil {
			t.Fatalf("GET %s: %v", c.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != c.wantStatus {
			t.Errorf("GET %s: status %d, want %d", c.path, resp.StatusCode, c.wantStatus)
		}
		if !strings.Contains(string(body), c.wantSubstr) {
			t.Errorf("GET %s: body does not contain %q. Got: %s", c.path, c.wantSubstr, body)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, c.wantCTPrefx) {
			t.Errorf("GET %s: Content-Type %q, want prefix %q", c.path, ct, c.wantCTPrefx)
		}
	}
}
