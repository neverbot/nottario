package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// A document keeps its frontmatter in storage and in every API
// response; only the rendered HTML leaves it out.
func TestApiDocs_WholeMarkdownRenderedWithoutFrontmatter(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, _ := identity.UpsertFromGithub(ctx, pool, 13521, "wm", "WM", "")
	key := []byte("test-session-key")
	p, _ := identity.CreateProject(ctx, pool, "WM", "", "", "", u.ID)
	path := "projects/" + p.ID.String() + "/notes/gamma.md"
	file := "---\ntitle: Gamma\ndescription: \"marker: yaml-only\"\n---\n\n# Gamma\n\nvisible text\n"
	zero := 0
	if _, err := docs.Write(ctx, pool, docs.WriteParams{
		Scope: docs.ScopeProject, ProjectID: &p.ID, Path: path,
		ContentMD: file, Message: "init", ExpectedVersion: &zero,
	}, docs.Authorship{UserID: &u.ID}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	sess, _ := identity.NewSession(ctx, pool, u.ID, "t", "127.0.0.1")
	cookie := &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key)}
	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, key, false)}))
	t.Cleanup(ts.Close)

	get := func(endpoint string, q url.Values) map[string]any {
		t.Helper()
		req, _ := http.NewRequest("GET", ts.URL+endpoint+"?"+q.Encode(), nil)
		req.AddCookie(cookie)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d %s", endpoint, resp.StatusCode, b)
		}
		var out map[string]any
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	q := url.Values{"scope": {"project"}, "project_id": {p.ID.String()}, "path": {path}}
	for name, got := range map[string]map[string]any{
		"read":         get("/api/docs/read", q),
		"read-version": get("/api/docs/read-version", url.Values{"scope": q["scope"], "project_id": q["project_id"], "path": q["path"], "version": {"1"}}),
	} {
		if got["content"] != file {
			t.Errorf("%s content is not the stored file:\n got %q\nwant %q", name, got["content"], file)
		}
		html, _ := got["content_html"].(string)
		if !strings.Contains(html, "visible text") {
			t.Errorf("%s html lost the body: %q", name, html)
		}
		if strings.Contains(html, "yaml-only") || strings.Contains(html, "title:") {
			t.Errorf("%s html renders the frontmatter: %q", name, html)
		}
	}
}

// Skill overrides are served exactly as written, not rebuilt from the
// parsed frontmatter.
func TestApiSkill_OverrideServedByteForByte(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	file := "---\n# a YAML comment the old rebuild would drop\ndescription: 'quoted: kept'\nname: nottario\n---\n\n\nOverride body.\n"
	if _, err := docs.Write(ctx, pool, docs.WriteParams{
		Scope: docs.ScopeGlobal, Path: "global/skills/extra.md", Kind: docs.KindSkill, ContentMD: file,
	}, docs.Authorship{}); err != nil {
		t.Fatalf("seed override: %v", err)
	}
	ts := httptest.NewServer(NewServer(Deps{Pool: pool, Resolver: identity.NewResolver(pool, []byte("k"), false)}))
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/skill/extra.md")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, []byte(file)) {
		t.Errorf("override not served byte for byte:\n got %q\nwant %q", got, file)
	}
}
