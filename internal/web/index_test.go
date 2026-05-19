package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIndexHandler_ReturnsHTML(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	IndexHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html...", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "<nottario-shell></nottario-shell>") {
		t.Errorf("body does not contain the shell element:\n%s", body)
	}
}
