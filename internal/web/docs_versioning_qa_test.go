package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// TestDocsVersioningConcurrencyAcrossHTTP is the QA smoke for the
// documents-versioning feature: two sessions racing to update the
// same doc must see a 409 on the loser carrying the live
// current_version, and the loser must succeed after re-reading.
//
// Covers the wire shape the agents and the web UI actually consume:
// REST through the real router, JSON in/out, Bearer-token auth.
func TestDocsVersioningConcurrencyAcrossHTTP(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()

	// Provision: user, token, project.
	u, _, err := identity.UpsertFromGithub(ctx, pool, 13001, "qa", "QA", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "QAProj", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	plaintext, _, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "qa", nil)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	srv := NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, []byte("test-session-key"), false),
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	auth := "Bearer " + plaintext
	docPath := "projects/" + p.ID.String() + "/context/qa-smoke.md"

	// 1) Write v1 (create). expected_version=0 because the doc does not exist.
	v0 := 0
	created, _ := writeDoc(t, ts, auth, writeDocBody{
		ProjectID:       p.ID.String(),
		Scope:           "project",
		Path:            docPath,
		ContentMD:       "# v1\n",
		Message:         "create",
		ExpectedVersion: &v0,
	})
	if created.CurrentVersion != 1 {
		t.Fatalf("expected v1 on create, got %d", created.CurrentVersion)
	}

	// 2) Two readers grab the same version=1.
	gotA := readDoc(t, ts, auth, p.ID.String(), docPath)
	gotB := readDoc(t, ts, auth, p.ID.String(), docPath)
	if gotA.CurrentVersion != 1 || gotB.CurrentVersion != 1 {
		t.Fatalf("expected both readers at v1, got A=%d B=%d", gotA.CurrentVersion, gotB.CurrentVersion)
	}

	// 3) Session A writes with expected_version=1 — succeeds (becomes v2).
	v1 := 1
	docAfterA, _ := writeDoc(t, ts, auth, writeDocBody{
		ProjectID:       p.ID.String(),
		Scope:           "project",
		Path:            docPath,
		ContentMD:       "# v2 (A)\n",
		Message:         "update A",
		ExpectedVersion: &v1,
	})
	if docAfterA.CurrentVersion != 2 {
		t.Fatalf("A's update should land v2, got %d", docAfterA.CurrentVersion)
	}

	// 4) Session B writes with the stale expected_version=1 — 409 with
	//    a structured body carrying current_version=2.
	_, respB := writeDoc(t, ts, auth, writeDocBody{
		ProjectID:       p.ID.String(),
		Scope:           "project",
		Path:            docPath,
		ContentMD:       "# v2 (B, stale)\n",
		Message:         "update B stale",
		ExpectedVersion: &v1,
	})
	if respB.StatusCode != http.StatusConflict {
		t.Fatalf("B stale: expected 409, got %d", respB.StatusCode)
	}
	var conflict struct {
		Error          string `json:"error"`
		CurrentVersion int    `json:"current_version"`
		Message        string `json:"message"`
	}
	if err := json.Unmarshal(respB.Body, &conflict); err != nil {
		t.Fatalf("decode conflict: %v body=%s", err, respB.Body)
	}
	if conflict.Error != "version_conflict" {
		t.Fatalf("expected error=version_conflict, got %q", conflict.Error)
	}
	if conflict.CurrentVersion != 2 {
		t.Fatalf("expected conflict.current_version=2, got %d", conflict.CurrentVersion)
	}
	if !strings.Contains(conflict.Message, "re-read") {
		t.Fatalf("expected human message about re-reading, got %q", conflict.Message)
	}

	// 5) B re-reads, sees v2, retries with expected_version=2 — succeeds (v3).
	gotBRetry := readDoc(t, ts, auth, p.ID.String(), docPath)
	if gotBRetry.CurrentVersion != 2 {
		t.Fatalf("B re-read should see v2, got %d", gotBRetry.CurrentVersion)
	}
	v2 := 2
	docAfterB, _ := writeDoc(t, ts, auth, writeDocBody{
		ProjectID:       p.ID.String(),
		Scope:           "project",
		Path:            docPath,
		ContentMD:       "# v3 (B retry)\n",
		Message:         "update B retry",
		ExpectedVersion: &v2,
	})
	if docAfterB.CurrentVersion != 3 {
		t.Fatalf("B retry should land v3, got %d", docAfterB.CurrentVersion)
	}
}

// --- tiny helpers, scoped to this test file ---

type writeDocBody struct {
	Scope           string `json:"scope"`
	ProjectID       string `json:"project_id"`
	Path            string `json:"path"`
	ContentMD       string `json:"content"`
	Message         string `json:"message"`
	ExpectedVersion *int   `json:"expected_version,omitempty"`
}

type docResp struct {
	ID             string `json:"id"`
	CurrentVersion int    `json:"current_version"`
}

type rawResp struct {
	StatusCode int
	Body       []byte
}

func writeDoc(t *testing.T, ts *httptest.Server, auth string, body writeDocBody) (docResp, rawResp) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", ts.URL+"/api/docs/write", bytes.NewReader(b))
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/docs/write: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	r := rawResp{StatusCode: resp.StatusCode, Body: raw}
	var dr docResp
	_ = json.Unmarshal(raw, &dr)
	return dr, r
}

func readDoc(t *testing.T, ts *httptest.Server, auth, projectID, path string) docResp {
	t.Helper()
	q := "?scope=project&project_id=" + projectID + "&path=" + path
	req, _ := http.NewRequest("GET", ts.URL+"/api/docs/read"+q, nil)
	req.Header.Set("Authorization", auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/docs/read: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/docs/read: status %d body %s", resp.StatusCode, raw)
	}
	var dr docResp
	if err := json.Unmarshal(raw, &dr); err != nil {
		t.Fatalf("decode read: %v body %s", err, raw)
	}
	return dr
}
