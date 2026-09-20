package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/identity"
)

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

type uploadResult struct {
	status int
	body   map[string]any
}

func putUpload(t *testing.T, rawURL string, body []byte, header map[string]string) uploadResult {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, rawURL, bytes.NewReader(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", rawURL, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return uploadResult{status: resp.StatusCode, body: out}
}

func (f *mcpFixture) requestUploadURL(t *testing.T, path string, expected int, sha string) map[string]any {
	t.Helper()
	var out map[string]any
	f.callJSON(t, "nottario.docs.upload_url", map[string]any{
		"project_id": f.projectID, "path": path,
		"expected_version": expected, "file_sha256": sha, "message": "upload",
	}, &out)
	return out
}

func (f *mcpFixture) pgx(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p, ok := f.pool.(*pgxpool.Pool)
	if !ok {
		t.Fatalf("fixture pool is %T", f.pool)
	}
	return p
}

func (f *mcpFixture) storedDoc(t *testing.T, path string) (*docs.Document, bool) {
	t.Helper()
	pid := uuid.MustParse(f.projectID)
	d, err := docs.Read(f.ctx, f.pgx(t), docs.ScopeProject, &pid, path)
	if errors.Is(err, docs.ErrNotFound) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("docs.Read %s: %v", path, err)
	}
	return d, true
}

func TestDocsUpload_HappyPathShapeTTLAndSingleUse(t *testing.T) {
	f := newMCPFixture(t, 16001, "upload-happy")
	body := []byte("---\ntitle: Plan\n---\n\n# Plan\n\nA literal \\n escape, a path C:\\x and ñ.\n")

	before := time.Now()
	out := f.requestUploadURL(t, "context/plan.md", 0, sha256Hex(body))
	uploadURL, _ := out["upload_url"].(string)
	if !strings.Contains(uploadURL, "/api/docs/upload?") {
		t.Fatalf("upload_url = %q", uploadURL)
	}
	if strings.Contains(uploadURL, "ntr_") {
		t.Error("upload_url contains token material")
	}
	// The response carries only what is specific to this call. How to
	// use the URL lives in the tool description, which the client holds
	// for the whole session instead of paying for it per upload.
	for _, k := range []string{"instructions", "method"} {
		if _, present := out[k]; present {
			t.Errorf("response still carries %q: %v", k, out)
		}
	}
	if len(out) != 3 {
		t.Errorf("response has %d fields, want upload_url + expires_at + expires_in_seconds: %v", len(out), out)
	}
	if out["expires_in_seconds"] != float64(300) {
		t.Errorf("expires_in_seconds = %v, want 300", out["expires_in_seconds"])
	}
	exp, err := time.Parse(time.RFC3339, out["expires_at"].(string))
	if err != nil {
		t.Fatalf("expires_at: %v", err)
	}
	if d := exp.Sub(before); d < 299*time.Second || d > 301*time.Second {
		t.Errorf("expires_at is %v after the request, want 5 minutes", d)
	}

	r := putUpload(t, uploadURL, body, nil)
	if r.status != http.StatusOK {
		t.Fatalf("PUT status %d body %v", r.status, r.body)
	}
	if r.body["current_version"] != float64(1) || r.body["path"] != "context/plan.md" {
		t.Errorf("ack = %v", r.body)
	}
	if _, echoed := r.body["content"]; echoed || len(r.body) > 3 {
		t.Errorf("ack is not slim: %v", r.body)
	}

	d, ok := f.storedDoc(t, "context/plan.md")
	if !ok {
		t.Fatal("document not stored")
	}
	if d.Title != "Plan" || d.ContentMD != string(body) {
		t.Errorf("stored title=%q content=%q", d.Title, d.ContentMD)
	}
	tokens, err := identity.ListProjectTokens(f.ctx, f.pgx(t), uuid.MustParse(f.projectID))
	if err != nil || len(tokens) != 1 {
		t.Fatalf("ListProjectTokens: %v (%d)", err, len(tokens))
	}
	if d.CreatedByTokenID == nil || *d.CreatedByTokenID != tokens[0].ID {
		t.Errorf("authorship token = %v, want %v", d.CreatedByTokenID, tokens[0].ID)
	}

	// The same URL a second time: the first write moved the version, so
	// the replay is refused and nothing changes.
	replay := putUpload(t, uploadURL, body, nil)
	if replay.status != http.StatusConflict || replay.body["current_version"] != float64(1) {
		t.Errorf("replay: status %d body %v, want 409 at version 1", replay.status, replay.body)
	}
}

func TestDocsUpload_RejectsTamperingAndMismatchedBodies(t *testing.T) {
	f := newMCPFixture(t, 16002, "upload-tamper")
	pool := f.pgx(t)
	body := []byte("# Original\n")
	uploadURL := f.requestUploadURL(t, "context/t.md", 0, sha256Hex(body))["upload_url"].(string)

	withQuery := func(mutate func(url.Values)) string {
		u, _ := url.Parse(uploadURL)
		q := u.Query()
		mutate(q)
		u.RawQuery = q.Encode()
		return u.String()
	}

	// Different bytes than the ones signed.
	if r := putUpload(t, uploadURL, []byte("# Something else\n"), nil); r.status != http.StatusBadRequest {
		t.Errorf("hash mismatch: status %d, want 400", r.status)
	}
	// Flipped signature.
	badSig := withQuery(func(q url.Values) {
		s := q.Get("sig")
		last := "0"
		if s[len(s)-1] == '0' {
			last = "1"
		}
		q.Set("sig", s[:len(s)-1]+last)
	})
	if r := putUpload(t, badSig, body, nil); r.status != http.StatusForbidden {
		t.Errorf("tampered sig: status %d, want 403", r.status)
	}
	// Redirected to another path with the original signature.
	evil := withQuery(func(q url.Values) { q.Set("path", "context/evil.md") })
	if r := putUpload(t, evil, body, nil); r.status != http.StatusForbidden {
		t.Errorf("tampered path: status %d, want 403", r.status)
	}
	// A valid Bearer token does not rescue a bad signature: the
	// endpoint does not consult tokens at all.
	plaintext, _, err := identity.IssueToken(f.ctx, pool, uuid.MustParse(f.userID), uuid.MustParse(f.projectID), "extra", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if r := putUpload(t, evil, body, map[string]string{"Authorization": "Bearer " + plaintext}); r.status != http.StatusForbidden {
		t.Errorf("tampered URL plus valid Bearer: status %d, want 403", r.status)
	}
	if _, ok := f.storedDoc(t, "context/evil.md"); ok {
		t.Error("a tampered path was written")
	}

	// Invalid UTF-8, correctly signed.
	bin := []byte{0xff, 0xfe, 'x'}
	binURL := f.requestUploadURL(t, "context/bin.md", 0, sha256Hex(bin))["upload_url"].(string)
	if r := putUpload(t, binURL, bin, nil); r.status != http.StatusBadRequest {
		t.Errorf("invalid UTF-8: status %d, want 400", r.status)
	}
	if _, ok := f.storedDoc(t, "context/bin.md"); ok {
		t.Error("invalid UTF-8 was stored")
	}

	// None of the failed attempts consumed the genuine URL.
	if r := putUpload(t, uploadURL, body, nil); r.status != http.StatusOK {
		t.Errorf("genuine upload after failures: status %d body %v", r.status, r.body)
	}

	// Tool-side validation.
	if msg := f.callExpectErr(t, "nottario.docs.upload_url", map[string]any{
		"project_id": f.projectID, "path": "context/x.md", "expected_version": 0, "file_sha256": "nope",
	}); !strings.Contains(msg, "file_sha256") {
		t.Errorf("bad hash accepted by the tool: %q", msg)
	}
	if msg := f.callExpectErr(t, "nottario.docs.upload_url", map[string]any{
		"project_id": f.projectID, "path": "context/x.md", "file_sha256": sha256Hex(body),
	}); !strings.Contains(msg, "expected_version") {
		t.Errorf("missing expected_version accepted: %q", msg)
	}
}

func TestDocsUpload_StaleVersion(t *testing.T) {
	f := newMCPFixture(t, 16003, "upload-stale")
	v1 := []byte("# v1\n")
	if r := putUpload(t, f.requestUploadURL(t, "context/s.md", 0, sha256Hex(v1))["upload_url"].(string), v1, nil); r.status != http.StatusOK {
		t.Fatalf("create: %d %v", r.status, r.body)
	}
	v2 := []byte("# v2 from the file\n")
	pending := f.requestUploadURL(t, "context/s.md", 1, sha256Hex(v2))["upload_url"].(string)

	// Someone else changes the document in the meantime.
	f.callJSON(t, "nottario.docs.write", map[string]any{
		"project_id": f.projectID, "path": "context/s.md", "content": "# concurrent\n", "expected_version": 1,
	}, nil)

	r := putUpload(t, pending, v2, nil)
	if r.status != http.StatusConflict || r.body["current_version"] != float64(2) {
		t.Errorf("stale upload: status %d body %v, want 409 at version 2", r.status, r.body)
	}
	if d, _ := f.storedDoc(t, "context/s.md"); d == nil || d.ContentMD != "# concurrent\n" {
		t.Error("a stale upload overwrote a newer version")
	}

	// And the tool refuses to sign for a version it can already see is stale.
	out := f.requestUploadURL(t, "context/s.md", 1, sha256Hex(v2))
	if out["error"] != "version_conflict" || out["current_version"] != float64(2) {
		t.Errorf("pre-check: %v", out)
	}
	if _, signed := out["upload_url"]; signed {
		t.Error("a URL was issued for a stale version")
	}
}

func TestDocsUpload_ExpiredURL(t *testing.T) {
	f := newMCPFixture(t, 16004, "upload-expired")
	body := []byte("# late\n")
	uploadURL := f.requestUploadURL(t, "context/late.md", 0, sha256Hex(body))["upload_url"].(string)

	uploadClock = func() time.Time { return time.Now().Add(docs.UploadTTL + time.Minute) }
	t.Cleanup(func() { uploadClock = time.Now })

	r := putUpload(t, uploadURL, body, nil)
	if r.status != http.StatusForbidden || !strings.Contains(r.body["error"].(string), "expired") {
		t.Errorf("expired URL: status %d body %v, want 403 expired", r.status, r.body)
	}
	if _, ok := f.storedDoc(t, "context/late.md"); ok {
		t.Error("an expired URL wrote the document")
	}
}

func TestDocsUpload_TokenScopeAndRevocation(t *testing.T) {
	f := newMCPFixture(t, 16005, "upload-revoke")
	pool := f.pgx(t)
	uid := uuid.MustParse(f.userID)

	// A token for this project cannot sign for another one, even though
	// the user (the instance admin) owns both.
	other, err := identity.CreateProject(f.ctx, pool, "Other", "", "", "", uid)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if msg := f.callExpectErr(t, "nottario.docs.upload_url", map[string]any{
		"project_id": other.ID.String(), "path": "context/x.md",
		"expected_version": 0, "file_sha256": sha256Hex([]byte("x")),
	}); !strings.Contains(msg, "token scoped") {
		t.Errorf("cross-project signing: %q", msg)
	}

	// Revoking the token kills URLs it already obtained.
	body := []byte("# after revoke\n")
	uploadURL := f.requestUploadURL(t, "context/r.md", 0, sha256Hex(body))["upload_url"].(string)
	tokens, err := identity.ListProjectTokens(f.ctx, pool, uuid.MustParse(f.projectID))
	if err != nil || len(tokens) != 1 {
		t.Fatalf("ListProjectTokens: %v", err)
	}
	if err := identity.RevokeToken(f.ctx, pool, tokens[0].ID, uid, true); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if r := putUpload(t, uploadURL, body, nil); r.status != http.StatusForbidden {
		t.Errorf("revoked token's URL: status %d body %v, want 403", r.status, r.body)
	}
	if _, ok := f.storedDoc(t, "context/r.md"); ok {
		t.Error("a revoked token's URL wrote the document")
	}
}

func TestDocsUpload_NoSigningKeyMeansNoUploads(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/docs/upload?p=x", strings.NewReader("x"))
	UploadDocHandler(nil, nil).ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503", rr.Code)
	}
}
