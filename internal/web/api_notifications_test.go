package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/notifications"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

type notifFixture struct {
	pool         *pgxpool.Pool
	me           *identity.User
	other        *identity.User
	projectID    uuid.UUID
	task         *tasks.Task
	cookie       *http.Cookie
	otherCookie  *http.Cookie
	deps         NotificationsDeps
	sessionKey   []byte
	notif        *notifications.Notifier
	handlerList  http.Handler
	handlerCount http.Handler
	handlerRead  http.Handler
	handlerAll   http.Handler
	handlerGetP  http.Handler
	handlerPatch http.Handler
}

func setupNotifFixture(t *testing.T, enabled bool) *notifFixture {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx := t.Context()
	me, _, err := identity.UpsertFromGithub(ctx, pool, 52001, "me", "Me", "")
	if err != nil {
		t.Fatalf("upsert me: %v", err)
	}
	other, _, err := identity.UpsertFromGithub(ctx, pool, 52002, "other", "Other", "")
	if err != nil {
		t.Fatalf("upsert other: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "N", "", "", "", me.ID)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	if err := identity.AddMembership(ctx, pool, other.ID, p.ID, roles[0].ID); err != nil {
		t.Fatalf("add membership: %v", err)
	}
	meID := me.ID
	tk, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID:      p.ID,
		Title:          "Do the thing",
		Type:           tasks.TypeTask,
		AssigneeUserID: &other.ID,
	}, tasks.Authorship{UserID: &meID})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	key := []byte("test-session-key")
	resolver := identity.NewResolver(pool, key, false)
	meSess, _ := identity.NewSession(ctx, pool, me.ID, "test", "127.0.0.1")
	otherSess, _ := identity.NewSession(ctx, pool, other.ID, "test", "127.0.0.1")
	deps := NotificationsDeps{Pool: pool, Resolver: resolver, Enabled: enabled}

	return &notifFixture{
		pool:         pool,
		me:           me,
		other:        other,
		projectID:    p.ID,
		task:         tk,
		deps:         deps,
		sessionKey:   key,
		cookie:       &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(meSess.ID, key)},
		otherCookie:  &http.Cookie{Name: identity.SessionCookieName, Value: identity.EncodeCookie(otherSess.ID, key)},
		notif:        notifications.New(pool, nil, nil, enabled),
		handlerList:  ListNotificationsHandler(deps),
		handlerCount: UnreadCountHandler(deps),
		handlerRead:  MarkReadHandler(deps),
		handlerAll:   MarkAllReadHandler(deps),
		handlerGetP:  GetPreferencesHandler(deps),
		handlerPatch: PatchPreferencesHandler(deps),
	}
}

// seed pushes one notification for the fixture's `me` user via the
// notifier (which owns dedup + prefs semantics).
func (f *notifFixture) seed(t *testing.T) {
	t.Helper()
	actor := f.other.ID
	f.notif.OnComment(t.Context(), f.task, &actor)
}

func TestNotifications_AnonymousIs401(t *testing.T) {
	f := setupNotifFixture(t, true)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	f.handlerList.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("list status = %d, want 401", rr.Code)
	}
}

func TestNotifications_ListReturnsOnlyCallersRows(t *testing.T) {
	f := setupNotifFixture(t, true)
	f.seed(t) // creates one row for `me`

	// meGET → 1 row
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	req.AddCookie(f.cookie)
	f.handlerList.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("me list = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	arr := body["notifications"].([]any)
	if len(arr) != 1 {
		t.Errorf("me list len = %d, want 1", len(arr))
	}

	// otherGET → 0 rows (comment on task where other IS actor, so no
	// self-notification generated)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	req.AddCookie(f.otherCookie)
	f.handlerList.ServeHTTP(rr, req)
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	arr = body["notifications"].([]any)
	if len(arr) != 0 {
		t.Errorf("other list len = %d, want 0", len(arr))
	}
}

func TestNotifications_UnreadCount(t *testing.T) {
	f := setupNotifFixture(t, true)
	f.seed(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/notifications/unread_count", nil)
	req.AddCookie(f.cookie)
	f.handlerCount.ServeHTTP(rr, req)
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if v, _ := body["unread"].(float64); v != 1 {
		t.Errorf("unread = %v, want 1", body["unread"])
	}
}

func TestNotifications_MarkReadScopedToCaller(t *testing.T) {
	f := setupNotifFixture(t, true)
	f.seed(t)

	// Grab the id from a list call.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	req.AddCookie(f.cookie)
	f.handlerList.ServeHTTP(rr, req)
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	arr := body["notifications"].([]any)
	id := arr[0].(map[string]any)["id"].(string)

	// Other tries to mark it read → SQL scope excludes their user_id → 0
	// marks, row stays unread.
	payload, _ := json.Marshal(map[string]any{"ids": []string{id}})
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/notifications/read", strings.NewReader(string(payload)))
	req.AddCookie(f.otherCookie)
	req.Header.Set("Content-Type", "application/json")
	f.handlerRead.ServeHTTP(rr, req)
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if v, _ := body["marked"].(float64); v != 0 {
		t.Errorf("other's mark = %v, want 0", body["marked"])
	}

	// Me marks it → 1.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/notifications/read", strings.NewReader(string(payload)))
	req.AddCookie(f.cookie)
	req.Header.Set("Content-Type", "application/json")
	f.handlerRead.ServeHTTP(rr, req)
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if v, _ := body["marked"].(float64); v != 1 {
		t.Errorf("my mark = %v, want 1", body["marked"])
	}
}

func TestNotifications_PatchPrefsRejectsUnknownKey(t *testing.T) {
	f := setupNotifFixture(t, true)
	payload, _ := json.Marshal(map[string]any{"pizza": true})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/me/notification_preferences", strings.NewReader(string(payload)))
	req.AddCookie(f.cookie)
	req.Header.Set("Content-Type", "application/json")
	f.handlerPatch.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestNotifications_PatchPrefsMerges(t *testing.T) {
	f := setupNotifFixture(t, true)
	payload, _ := json.Marshal(map[string]any{"task_commented": false})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/me/notification_preferences", strings.NewReader(string(payload)))
	req.AddCookie(f.cookie)
	req.Header.Set("Content-Type", "application/json")
	f.handlerPatch.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	// Confirm via GET: task_commented false, others still true.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/me/notification_preferences", nil)
	req.AddCookie(f.cookie)
	f.handlerGetP.ServeHTTP(rr, req)
	var body map[string]bool
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body["task_commented"] != false {
		t.Errorf("task_commented = %v, want false", body["task_commented"])
	}
	if body["task_assigned"] != true {
		t.Errorf("task_assigned = %v, want true (default)", body["task_assigned"])
	}
	// Confirm the JSONB blob has the merged value.
	raw, err := dbq.New(f.pool).GetPreferences(t.Context(), f.me.ID)
	if err != nil {
		t.Fatalf("get prefs: %v", err)
	}
	// Postgres normalises jsonb whitespace, so match with a space.
	if !strings.Contains(string(raw), "\"task_commented\": false") {
		t.Errorf("stored prefs missing merged value: %s", raw)
	}
}

func TestNotifications_DisabledReturnsEmptyAnd501(t *testing.T) {
	f := setupNotifFixture(t, false /* disabled */)
	// GET list returns 200 with disabled:true and empty array.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	req.AddCookie(f.cookie)
	f.handlerList.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("disabled list = %d, want 200", rr.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body["disabled"] != true {
		t.Errorf("disabled flag = %v, want true", body["disabled"])
	}
	if arr := body["notifications"].([]any); len(arr) != 0 {
		t.Errorf("disabled list len = %d, want 0", len(arr))
	}

	// POST read returns 501.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/notifications/read",
		strings.NewReader(`{"ids":[]}`))
	req.AddCookie(f.cookie)
	req.Header.Set("Content-Type", "application/json")
	f.handlerRead.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("disabled mark-read = %d, want 501", rr.Code)
	}
}
