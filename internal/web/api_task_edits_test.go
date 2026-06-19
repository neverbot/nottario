package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/testutil"
)

// TestApiTaskEdits_TextAndComments exercises the new human-edit
// endpoints: PATCH /tasks/{id}/text, PATCH /tasks/{id}/comments/{cid}
// and DELETE /tasks/{id}/comments/{cid}.
//
// Covered branches:
//   - Member edits title + description: 200 + edited_at + edited_by set.
//   - Non-admin sets target_role: 403.
//   - Admin sets target_role: 200.
//   - Comment author edits own comment: 200 + edited markers.
//   - Comment author edits another's comment: 403.
//   - Admin edits another's comment: 200.
//   - Comment author deletes own comment: 204.
//   - Non-author non-admin deletes: 403.
//   - Optimistic concurrency: stale expected_updated_at → 409.
func TestApiTaskEdits_TextAndComments(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()

	// Admin / project owner.
	admin, _, err := identity.UpsertFromGithub(ctx, pool, 14001, "edit-admin", "Admin", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub admin: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET is_admin = TRUE WHERE id = $1", admin.ID); err != nil {
		t.Fatalf("promote admin: %v", err)
	}
	proj, err := identity.CreateProject(ctx, pool, "EditFlow", "", "", "", admin.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	roles, _ := identity.ListRoles(ctx, pool, proj.ID)
	if len(roles) < 2 {
		t.Fatalf("expected at least two seeded roles")
	}
	if err := identity.AddMembership(ctx, pool, admin.ID, proj.ID, roles[0].ID); err != nil {
		t.Fatalf("AddMembership admin: %v", err)
	}
	adminToken, _, _ := identity.IssueToken(ctx, pool, admin.ID, proj.ID, "admin-tok", nil)

	// Regular member (not admin).
	member, _, _ := identity.UpsertFromGithub(ctx, pool, 14002, "edit-member", "Member", "")
	_ = identity.AddMembership(ctx, pool, member.ID, proj.ID, roles[0].ID)
	memberToken, _, _ := identity.IssueToken(ctx, pool, member.ID, proj.ID, "member-tok", nil)

	// Another regular member.
	other, _, _ := identity.UpsertFromGithub(ctx, pool, 14003, "edit-other", "Other", "")
	_ = identity.AddMembership(ctx, pool, other.ID, proj.ID, roles[0].ID)
	otherToken, _, _ := identity.IssueToken(ctx, pool, other.ID, proj.ID, "other-tok", nil)

	srv := NewServer(Deps{
		Pool:     pool,
		Resolver: identity.NewResolver(pool, []byte("test-session-key"), false),
	})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	authAdmin := "Bearer " + adminToken
	authMember := "Bearer " + memberToken
	authOther := "Bearer " + otherToken
	pid := proj.ID.String()

	// Seed a task as the admin.
	var created map[string]any
	doJSON(t, "POST", ts.URL+"/api/projects/"+pid+"/tasks", authAdmin,
		[]byte(`{"title":"original","description":"hello","type":"task"}`), &created)
	taskID := created["id"].(string)
	originalUpdatedAt := created["updated_at"].(string)

	// --- Member edits title + description: 200 + edited markers. ---
	editBody := fmt.Sprintf(`{"title":"edited title","description":"new body","expected_updated_at":%q}`,
		originalUpdatedAt)
	var afterEdit map[string]any
	doJSON(t, "PATCH", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/text", authMember,
		[]byte(editBody), &afterEdit)
	if afterEdit["title"] != "edited title" || afterEdit["description"] != "new body" {
		t.Fatalf("edit didn't apply: %+v", afterEdit)
	}
	if afterEdit["edited_at"] == nil {
		t.Fatalf("edited_at not set after text edit")
	}
	if afterEdit["edited_by_user_id"] != member.ID.String() {
		t.Fatalf("edited_by_user_id = %v, want %s", afterEdit["edited_by_user_id"], member.ID)
	}

	// --- Member tries to set target_role: 403. ---
	roleBody := fmt.Sprintf(`{"target_role_id":%q,"expected_updated_at":%q}`,
		roles[1].ID.String(), afterEdit["updated_at"])
	if r := doRaw(t, "PATCH", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/text", authMember,
		[]byte(roleBody)); r.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin role edit: status=%d body=%s", r.StatusCode, r.Body)
	}

	// --- Admin sets target_role: 200. ---
	var afterRole map[string]any
	doJSON(t, "PATCH", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/text", authAdmin,
		[]byte(roleBody), &afterRole)
	if afterRole["target_role_id"] != roles[1].ID.String() {
		t.Fatalf("admin role edit failed: %+v", afterRole)
	}

	// --- Stale expected_updated_at: 409. ---
	staleBody := fmt.Sprintf(`{"title":"x","expected_updated_at":%q}`, originalUpdatedAt)
	if r := doRaw(t, "PATCH", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/text", authMember,
		[]byte(staleBody)); r.StatusCode != http.StatusConflict {
		t.Fatalf("stale edit: status=%d body=%s", r.StatusCode, r.Body)
	}

	// --- Seed a comment as `member`. ---
	var comment map[string]any
	doJSON(t, "POST", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/comments", authMember,
		[]byte(`{"body":"first comment"}`), &comment)
	commentID := comment["id"].(string)
	commentUpdatedAt := comment["updated_at"].(string)

	// --- Author edits own comment: 200 + edited markers. ---
	editCommentBody := fmt.Sprintf(`{"body":"edited comment","expected_updated_at":%q}`, commentUpdatedAt)
	var afterCommentEdit map[string]any
	doJSON(t, "PATCH", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/comments/"+commentID, authMember,
		[]byte(editCommentBody), &afterCommentEdit)
	if afterCommentEdit["body"] != "edited comment" {
		t.Fatalf("comment edit didn't apply: %+v", afterCommentEdit)
	}
	if afterCommentEdit["edited_at"] == nil {
		t.Fatalf("comment edited_at not set")
	}
	if afterCommentEdit["edited_by_user_id"] != member.ID.String() {
		t.Fatalf("comment edited_by_user_id = %v", afterCommentEdit["edited_by_user_id"])
	}

	// --- Other (non-admin, non-author) edits the comment: 403. ---
	commentNowUpdated := afterCommentEdit["updated_at"].(string)
	otherEditBody := fmt.Sprintf(`{"body":"forbidden","expected_updated_at":%q}`, commentNowUpdated)
	if r := doRaw(t, "PATCH", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/comments/"+commentID, authOther,
		[]byte(otherEditBody)); r.StatusCode != http.StatusForbidden {
		t.Fatalf("other edits comment: status=%d body=%s", r.StatusCode, r.Body)
	}

	// --- Admin edits another's comment: 200. ---
	adminEditBody := fmt.Sprintf(`{"body":"by admin","expected_updated_at":%q}`, commentNowUpdated)
	var afterAdminEdit map[string]any
	doJSON(t, "PATCH", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/comments/"+commentID, authAdmin,
		[]byte(adminEditBody), &afterAdminEdit)
	if afterAdminEdit["body"] != "by admin" {
		t.Fatalf("admin edit didn't apply: %+v", afterAdminEdit)
	}

	// --- Other (non-admin, non-author) deletes comment: 403. ---
	if r := doRaw(t, "DELETE", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/comments/"+commentID, authOther,
		nil); r.StatusCode != http.StatusForbidden {
		t.Fatalf("other deletes comment: status=%d body=%s", r.StatusCode, r.Body)
	}

	// --- Author deletes own comment: 204. ---
	if r := doRaw(t, "DELETE", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/comments/"+commentID, authMember,
		nil); r.StatusCode != http.StatusNoContent {
		t.Fatalf("author delete: status=%d body=%s", r.StatusCode, r.Body)
	}

	// --- Subsequent GET of the task should not list it. ---
	var afterDelete map[string]any
	doJSON(t, "GET", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID, authAdmin, nil, &afterDelete)
	comments, _ := afterDelete["comments"].([]any)
	for _, c := range comments {
		if c.(map[string]any)["id"].(string) == commentID {
			t.Fatalf("deleted comment still listed: %+v", c)
		}
	}

	// --- Stale comment edit: insert a second comment, mutate it, then
	// try to edit with the original updated_at. Should 409. ---
	var c2 map[string]any
	doJSON(t, "POST", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/comments", authMember,
		[]byte(`{"body":"c2"}`), &c2)
	c2ID := c2["id"].(string)
	c2Original := c2["updated_at"].(string)
	// First edit succeeds.
	doJSON(t, "PATCH", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/comments/"+c2ID, authMember,
		[]byte(fmt.Sprintf(`{"body":"c2-edited","expected_updated_at":%q}`, c2Original)), &c2)
	// Second edit with the original stamp must 409.
	if r := doRaw(t, "PATCH", ts.URL+"/api/projects/"+pid+"/tasks/"+taskID+"/comments/"+c2ID, authMember,
		[]byte(fmt.Sprintf(`{"body":"c2-stale","expected_updated_at":%q}`, c2Original))); r.StatusCode != http.StatusConflict {
		t.Fatalf("stale comment edit: status=%d body=%s", r.StatusCode, r.Body)
	}

	// Touch user/uuid/time imports to silence the linter when the test
	// file is regenerated.
	_ = uuid.Nil
	_ = json.Valid
	_ = time.Now
}
