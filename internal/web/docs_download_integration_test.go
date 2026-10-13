package web

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/identity"
)

func getDownload(t *testing.T, rawURL string, header map[string]string) (int, string, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, rawURL, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", rawURL, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw), resp.Header
}

func (f *mcpFixture) requestDownloadURL(t *testing.T, path string) map[string]any {
	t.Helper()
	var out map[string]any
	f.callJSON(t, "nottario.docs.download_url", map[string]any{"project_id": f.projectID, "path": path}, &out)
	return out
}

// Download, edit locally, upload back: the round trip that keeps a
// document out of the agent's context.
func TestDocsDownload_RoundTripWithUpload(t *testing.T) {
	f := newMCPFixture(t, 16101, "download-round")
	v1 := "---\ntitle: Plan\n---\n\n# Plan\n\nñ and a \\n escape.\n"
	f.callJSON(t, "nottario.docs.write", map[string]any{
		"project_id": f.projectID, "path": "context/plan.md", "content": v1, "expected_version": 0,
	}, nil)

	out := f.requestDownloadURL(t, "context/plan.md")
	u, _ := out["download_url"].(string)
	if !strings.Contains(u, "/api/docs/download?") || strings.Contains(u, "ntr_") {
		t.Fatalf("download_url = %q", u)
	}
	if out["version"] != float64(1) || out["expires_in_seconds"] != float64(300) || len(out) != 3 {
		t.Errorf("response = %v", out)
	}
	status, body, h := getDownload(t, u, nil)
	if status != http.StatusOK || body != v1 || h.Get("X-Nottario-Version") != "1" {
		t.Fatalf("GET: %d %q version %q", status, body, h.Get("X-Nottario-Version"))
	}
	// Reading is idempotent: the URL works again until it expires.
	if status, again, _ := getDownload(t, u, nil); status != http.StatusOK || again != v1 {
		t.Errorf("second GET: %d", status)
	}

	edited := []byte(strings.Replace(body, "# Plan", "# Plan, edited", 1))
	up := f.requestUploadURL(t, "context/plan.md", int(out["version"].(float64)), sha256Hex(edited))
	if r := putUpload(t, up["upload_url"].(string), edited, nil); r.status != http.StatusOK {
		t.Fatalf("upload back: %d %v", r.status, r.body)
	}

	// The old URL still serves the version it was signed for.
	if _, old, h := getDownload(t, u, nil); old != v1 || h.Get("X-Nottario-Version") != "1" {
		t.Errorf("signed version 1, got %q at %q", old, h.Get("X-Nottario-Version"))
	}
}

func TestDocsDownload_RejectsTamperingExpiryAndMissingDocs(t *testing.T) {
	f := newMCPFixture(t, 16102, "download-reject")
	f.callJSON(t, "nottario.docs.write", map[string]any{
		"project_id": f.projectID, "path": "context/a.md", "content": "# a\n", "expected_version": 0,
	}, nil)
	f.callJSON(t, "nottario.docs.write", map[string]any{
		"project_id": f.projectID, "path": "context/b.md", "content": "# b\n", "expected_version": 0,
	}, nil)
	u := f.requestDownloadURL(t, "context/a.md")["download_url"].(string)

	if status, _, _ := getDownload(t, strings.Replace(u, "context%2Fa.md", "context%2Fb.md", 1), nil); status != http.StatusForbidden {
		t.Errorf("path swapped: %d, want 403", status)
	}
	if msg := f.callExpectErr(t, "nottario.docs.download_url", map[string]any{"project_id": f.projectID, "path": "context/none.md"}); !strings.Contains(msg, "not found") {
		t.Errorf("missing doc: %q", msg)
	}

	downloadClock = func() time.Time { return time.Now().Add(docs.DownloadTTL + time.Minute) }
	status, body, _ := getDownload(t, u, nil)
	downloadClock = time.Now
	if status != http.StatusForbidden || !strings.Contains(body, "expired") {
		t.Errorf("expired: %d %s", status, body)
	}
}

func TestDocsDownload_TokenScopeAndRevocation(t *testing.T) {
	f := newMCPFixture(t, 16103, "download-revoke")
	pool := f.pgx(t)
	uid := uuid.MustParse(f.userID)
	other, err := identity.CreateProject(f.ctx, pool, "Other", "", "", "", uid)
	if err != nil {
		t.Fatal(err)
	}
	if msg := f.callExpectErr(t, "nottario.docs.download_url", map[string]any{
		"project_id": other.ID.String(), "path": "context/x.md",
	}); !strings.Contains(msg, "token scoped") {
		t.Errorf("cross-project signing: %q", msg)
	}

	f.callJSON(t, "nottario.docs.write", map[string]any{
		"project_id": f.projectID, "path": "context/r.md", "content": "# secret\n", "expected_version": 0,
	}, nil)
	u := f.requestDownloadURL(t, "context/r.md")["download_url"].(string)
	tokens, err := identity.ListProjectTokens(f.ctx, pool, uuid.MustParse(f.projectID))
	if err != nil || len(tokens) != 1 {
		t.Fatalf("ListProjectTokens: %v", err)
	}
	if err := identity.RevokeToken(f.ctx, pool, tokens[0].ID, uid, true); err != nil {
		t.Fatal(err)
	}
	if status, body, _ := getDownload(t, u, nil); status != http.StatusForbidden || strings.Contains(body, "secret") {
		t.Errorf("revoked token's URL: %d %q", status, body)
	}
}
