package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

type bodyLimitFixture struct {
	ts        *httptest.Server
	token     string
	projectID string
}

func newBodyLimitFixture(t *testing.T, githubID int64) bodyLimitFixture {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx := context.Background()
	u, _, err := identity.UpsertFromGithub(ctx, pool, githubID, "bodylimit", "Body Limit", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "BodyLimit", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	token, _, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "bl", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, []byte("test-session-key"), false)}))
	t.Cleanup(ts.Close)
	return bodyLimitFixture{ts: ts, token: token, projectID: p.ID.String()}
}

// docBody builds a docs.write JSON body whose content is n bytes long.
func (f bodyLimitFixture) docBody(path string, n int) string {
	return `{"scope":"project","project_id":"` + f.projectID + `","path":"` + path +
		`","content":"` + strings.Repeat("a", n) + `","expected_version":0}`
}

func TestBodyLimit_DeclaredOversizeIsRefusedBeforeReading(t *testing.T) {
	f := newBodyLimitFixture(t, 15101)
	body := f.docBody("context/huge.md", maxRequestBodyBytes)

	for _, path := range []string{"/api/docs/write", "/mcp"} {
		req, _ := http.NewRequest(http.MethodPost, f.ts.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+f.token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusRequestEntityTooLarge {
			t.Errorf("POST %s with %d-byte body: status %d, want 413", path, len(body), resp.StatusCode)
		}
	}
	assertNotStored(t, f, "context/huge.md")
}

func TestBodyLimit_UndeclaredOversizeIsCutOff(t *testing.T) {
	f := newBodyLimitFixture(t, 15102)
	body := f.docBody("context/chunked.md", maxRequestBodyBytes)

	// No declared length: net/http sends it chunked, so only the
	// MaxBytesReader stands between it and the handler.
	req, _ := http.NewRequest(http.MethodPost, f.ts.URL+"/api/docs/write", io.NopCloser(strings.NewReader(body)))
	req.ContentLength = -1
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST chunked: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		t.Errorf("chunked oversized body accepted with status %d", resp.StatusCode)
	}
	assertNotStored(t, f, "context/chunked.md")
}

func TestBodyLimit_LargeRealisticDocumentStillAccepted(t *testing.T) {
	f := newBodyLimitFixture(t, 15103)
	// 1 MiB of markdown is already far past any real document; it must
	// go through untouched.
	body := f.docBody("context/big-but-fine.md", 1<<20)
	req, _ := http.NewRequest(http.MethodPost, f.ts.URL+"/api/docs/write", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("1 MiB document: status %d, want 200", resp.StatusCode)
	}
}

func assertNotStored(t *testing.T, f bodyLimitFixture, path string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet,
		f.ts.URL+"/api/docs/read?scope=project&project_id="+f.projectID+"&path="+path, nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET read: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Errorf("%s was stored despite the body limit", path)
	}
}
