package web

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"
)

// `nottario.search` and `nottario.skill.*` are small families and
// share a test file. Search hits a seeded task via the unified FTS
// query; skill list/read hit the embedded bundle (no DB writes).
func TestMCP_Search(t *testing.T) {
	f := newMCPFixture(t, 13340, "search-mcp")

	// Seed an arch node containing the query term so search has
	// something to return without depending on task seeding.
	f.callJSON(t, "nottario.arch.upsert_node", map[string]any{
		"project_id": f.projectID,
		"slug":       "thingamajig", "kind": "service", "name": "thingamajig",
		"description": "the thingamajig service handles widget orchestration",
	}, nil)

	var hits struct {
		Hits []map[string]any `json:"hits"`
	}
	f.callJSON(t, "nottario.search", map[string]any{
		"project_id": f.projectID,
		"query":      "thingamajig",
	}, &hits)
	if len(hits.Hits) == 0 {
		t.Error("search returned 0 hits for known content")
	}
	// Slim default: no raw description fallback on any hit.
	for i, h := range hits.Hits {
		if _, ok := h["description"]; ok {
			t.Errorf("slim search hit %d must omit 'description', got %+v", i, h)
		}
	}

	// verbose=true brings the raw description back.
	var verbose struct {
		Hits []map[string]any `json:"hits"`
	}
	f.callJSON(t, "nottario.search", map[string]any{
		"project_id": f.projectID,
		"query":      "thingamajig",
		"verbose":    true,
	}, &verbose)
	hasDescField := false
	for _, h := range verbose.Hits {
		if _, ok := h["description"]; ok {
			hasDescField = true
			break
		}
	}
	if !hasDescField {
		t.Errorf("verbose search must surface 'description', got hits=%+v", verbose.Hits)
	}
}

func TestMCP_SkillInstall(t *testing.T) {
	f := newMCPFixture(t, 13341, "skill")

	var resp struct {
		DownloadURL   string         `json:"download_url"`
		Format        string         `json:"format"`
		BundleVersion string         `json:"bundle_version"`
		Install       map[string]any `json:"install"`
	}
	f.callJSON(t, "nottario.skill.install", map[string]any{}, &resp)

	if resp.Format != "zip" {
		t.Errorf("format = %q, want zip", resp.Format)
	}
	if !strings.HasPrefix(resp.BundleVersion, "sha256:") || len(resp.BundleVersion) < 16 {
		t.Errorf("bundle_version looks wrong: %q", resp.BundleVersion)
	}
	if resp.DownloadURL == "" || !strings.Contains(resp.DownloadURL, "sig=") || !strings.Contains(resp.DownloadURL, "exp=") {
		t.Errorf("download_url missing sig/exp: %q", resp.DownloadURL)
	}
	for _, k := range []string{"name", "preferred_dir", "fallback_dir", "instructions"} {
		if _, ok := resp.Install[k]; !ok {
			t.Errorf("install map missing %q: %+v", k, resp.Install)
		}
	}

	// Fetch the signed URL — it should return a valid zip without any
	// Authorization header.
	httpResp, err := http.Get(resp.DownloadURL)
	if err != nil {
		t.Fatalf("GET %s: %v", resp.DownloadURL, err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != 200 {
		t.Fatalf("signed URL returned %d, want 200", httpResp.StatusCode)
	}
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("response is not a valid zip: %v", err)
	}
	assertBundleVerifiable(t, zr, resp.BundleVersion)

	// Same content, same bytes: a second download is identical.
	again, err := http.Get(resp.DownloadURL)
	if err != nil {
		t.Fatalf("GET again: %v", err)
	}
	body2, _ := io.ReadAll(again.Body)
	_ = again.Body.Close()
	if !bytes.Equal(body, body2) {
		t.Error("two downloads of the same bundle differ byte-wise")
	}

	// Tampered signature → 401.
	tampered := strings.Replace(resp.DownloadURL, "sig=", "sig=ff", 1)
	httpResp, err = http.Get(tampered)
	if err != nil {
		t.Fatalf("GET tampered: %v", err)
	}
	_ = httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusUnauthorized {
		t.Errorf("tampered URL returned %d, want 401", httpResp.StatusCode)
	}
}

// assertBundleVerifiable checks the bundle the way an agent does from
// disk: SHA256SUMS hashes to bundle_version, and it lists every other
// file in the zip with its correct digest.
func assertBundleVerifiable(t *testing.T, zr *zip.Reader, bundleVersion string) {
	t.Helper()
	contents := map[string][]byte{}
	for _, e := range zr.File {
		rc, err := e.Open()
		if err != nil {
			t.Fatalf("open %s: %v", e.Name, err)
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		contents[e.Name] = data
	}
	if _, ok := contents["skill.md"]; !ok {
		t.Error("zip is missing skill.md")
	}
	manifest, ok := contents["SHA256SUMS"]
	if !ok {
		t.Fatal("zip is missing SHA256SUMS")
	}
	sum := sha256.Sum256(manifest)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != bundleVersion {
		t.Errorf("sha256(SHA256SUMS) = %s, bundle_version = %s", got, bundleVersion)
	}
	listed := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(string(manifest), "\n"), "\n") {
		digest, name, found := strings.Cut(line, "  ")
		if !found {
			t.Fatalf("manifest line is not sha256sum format: %q", line)
		}
		data, ok := contents[name]
		if !ok {
			t.Errorf("manifest lists %s, which is not in the zip", name)
			continue
		}
		fileSum := sha256.Sum256(data)
		if hex.EncodeToString(fileSum[:]) != digest {
			t.Errorf("manifest digest for %s does not match its content", name)
		}
		listed[name] = true
	}
	for name := range contents {
		if name != "SHA256SUMS" && !listed[name] {
			t.Errorf("%s is in the zip but not in SHA256SUMS", name)
		}
	}
}
