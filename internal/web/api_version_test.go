package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/selfupdate"
	"github.com/neverbot/nottario/internal/testutil"
	"github.com/neverbot/nottario/internal/version"
)

// updateAvailable is the pure-logic gate that lets us test the "how"
// without spinning up the whole HTTP stack.

func TestUpdateAvailable_BlanksAreUnknown(t *testing.T) {
	if updateAvailable("", "abc") {
		t.Errorf(`updateAvailable("", "abc") = true, want false`)
	}
	if updateAvailable("abc", "") {
		t.Errorf(`updateAvailable("abc", "") = true, want false`)
	}
	if updateAvailable("none", "abc") {
		// "none" is the version-package sentinel for dev builds without
		// ldflags injection — must not trigger a banner.
		t.Errorf(`updateAvailable("none", "abc") = true, want false`)
	}
}

func TestUpdateAvailable_DifferingShas(t *testing.T) {
	if !updateAvailable("aaa111", "bbb222") {
		t.Errorf(`updateAvailable("aaa111", "bbb222") = false, want true`)
	}
	// Case-insensitive equality (GitHub lowercase, our ldflags could be
	// either — we keep them consistent but the check tolerates both).
	if updateAvailable("ABC123", "abc123") {
		t.Errorf(`updateAvailable("ABC123", "abc123") = true, want false (equal)`)
	}
}

// Real production shape: CI stamps `internal/version.Commit` via
// `git rev-parse --short HEAD` (7 chars) but the GitHub commits API
// returns the full 40-char sha. The two sides must compare equal
// when the short is a prefix of the full, otherwise the update
// banner sticks ON forever on every self-hoster.
func TestUpdateAvailable_ShortFullMix(t *testing.T) {
	short := "abc1234"
	fullSame := "abc1234def5678901234567890123456789012345"
	fullOther := "def56789012345678901234567890123456789012"
	if updateAvailable(short, fullSame) {
		t.Errorf("short vs full-same-prefix reported update available, want false")
	}
	if updateAvailable(fullSame, short) {
		t.Errorf("full vs short-same-prefix reported update available, want false")
	}
	if !updateAvailable(short, fullOther) {
		t.Errorf("short vs full-differing-prefix reported no update, want true")
	}
	// Case-insensitive across the mixed lengths.
	if updateAvailable("ABC1234", fullSame) {
		t.Errorf("uppercase short vs lowercase full-same-prefix reported update available, want false")
	}
}

// End-to-end: anonymous call to the endpoint returns 401.
func TestVersionStatusHandler_AnonymousIs401(t *testing.T) {
	pool := testutil.NewPool(t)
	key := []byte("test-session-key")
	resolver := identity.NewResolver(pool, key, false)

	h := VersionStatusHandler(VersionStatusDeps{
		Resolver: resolver,
		State:    nil,
		Upstream: "neverbot/nottario",
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/version/status", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

// Non-admin path: the FIRST user in the DB is auto-admin (see
// identity.UpsertFromGithub), so we create two users and use the
// second one (guaranteed non-admin). Non-admin must never see a
// `latest` block nor `update_available: true`, even when the poller
// state carries a differing sha.
func TestVersionStatusHandler_NonAdminNeverSeesLatest(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	// First user is auto-admin — burn one so the second is non-admin.
	if _, _, err := identity.UpsertFromGithub(ctx, pool, 40001, "vsu-admin", "Admin", ""); err != nil {
		t.Fatalf("upsert admin: %v", err)
	}
	user, _, err := identity.UpsertFromGithub(ctx, pool, 41041, "vsu-user", "User", "")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	if user.IsAdmin {
		t.Fatalf("second user unexpectedly is_admin — seed order changed?")
	}
	sess, _ := identity.NewSession(ctx, pool, user.ID, "test", "127.0.0.1")
	key := []byte("test-session-key")
	cookie := &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key)}
	resolver := identity.NewResolver(pool, key, false)

	h := VersionStatusHandler(VersionStatusDeps{
		Resolver: resolver,
		State:    &selfupdate.State{},
		Upstream: "neverbot/nottario",
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/version/status", nil)
	req.AddCookie(cookie)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["update_available"] != false {
		t.Errorf("update_available = %v, want false for non-admin", body["update_available"])
	}
	if body["latest"] != nil {
		t.Errorf("latest = %v, want nil for non-admin", body["latest"])
	}
	if body["check_enabled"] != true {
		t.Errorf("check_enabled = %v, want true (state non-nil)", body["check_enabled"])
	}
	running, ok := body["running"].(map[string]any)
	if !ok {
		t.Fatalf("running is not an object: %v", body["running"])
	}
	if running["sha"] != version.Commit {
		t.Errorf("running.sha = %v, want %q", running["sha"], version.Commit)
	}
}

// Test the disabled-poller path: State=nil ⇒ check_enabled=false.
func TestVersionStatusHandler_StateNilReportsCheckDisabled(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	user, _, _ := identity.UpsertFromGithub(ctx, pool, 41042, "vsu-user2", "User", "")
	sess, _ := identity.NewSession(ctx, pool, user.ID, "test", "127.0.0.1")
	key := []byte("test-session-key")
	cookie := &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(sess.ID, key)}
	resolver := identity.NewResolver(pool, key, false)

	h := VersionStatusHandler(VersionStatusDeps{
		Resolver: resolver,
		State:    nil,
		Upstream: "neverbot/nottario",
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/version/status", nil)
	req.AddCookie(cookie)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["check_enabled"] != false {
		t.Errorf("check_enabled = %v, want false with State=nil", body["check_enabled"])
	}
	if body["update_available"] != false {
		t.Errorf("update_available = %v, want false with State=nil", body["update_available"])
	}
	if body["latest"] != nil {
		t.Errorf("latest = %v, want nil with State=nil", body["latest"])
	}
}

// Keep imports pinned even though the current tests do not consume
// them directly — the pattern will grow as we add integration
// coverage for the admin + poller-populated path.
var _ = time.Time{}
