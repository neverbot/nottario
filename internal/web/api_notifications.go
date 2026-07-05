package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/notifications"
	"github.com/neverbot/nottario/internal/tasks"
)

// NotificationsDeps wires the personal-inbox endpoints. Every handler
// operates on caller.UserID only; there is no cross-user access and
// admin does not bypass. The Enabled flag mirrors the NOTIFICATIONS_
// ENABLED env var: when false, reads return an empty payload with
// disabled=true and writes fail 501.
type NotificationsDeps struct {
	Pool     *pgxpool.Pool
	Resolver *identity.Resolver
	Enabled  bool
}

// AllowedPrefKeys is the closed set of preference toggles that the
// PATCH endpoint accepts. Anything else is rejected 400 rather than
// silently stored — schema drift protection.
var AllowedPrefKeys = map[string]struct{}{
	notifications.KindTaskAssigned:  {},
	notifications.KindTaskCommented: {},
	notifications.KindTaskClosed:    {},
}

func (d NotificationsDeps) caller(r *http.Request) (identity.Caller, bool) {
	if c, ok := d.Resolver.ResolveSession(r); ok {
		return c, true
	}
	return d.Resolver.ResolveToken(r)
}

// ListNotificationsHandler returns the caller's most recent 20 (or
// `limit`) notifications, keyset-paginated by (created_at DESC, id
// ASC). Each row is hydrated with the referenced task title/project
// and the actor's minimal profile so the drawer doesn't have to
// round-trip N times.
func ListNotificationsHandler(d NotificationsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if !d.Enabled {
			writeJSON(w, http.StatusOK, map[string]any{
				"notifications": []any{},
				"disabled":      true,
				"next_after":    nil,
			})
			return
		}
		limit := 20
		if s := r.URL.Query().Get("limit"); s != "" {
			var v int
			if _, err := fmt.Sscanf(s, "%d", &v); err == nil && v > 0 && v <= 100 {
				limit = v
			}
		}
		params := dbq.ListNotificationsParams{
			UserID: c.UserID,
			Lim:    int32(limit + 1),
		}
		if s := r.URL.Query().Get("after_created_at"); s != "" {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				params.AfterCreatedAt = pgtype.Timestamptz{Time: t, Valid: true}
			}
		}
		if s := r.URL.Query().Get("after_id"); s != "" {
			if id, err := uuid.Parse(s); err == nil {
				params.AfterID = &id
			}
		}
		q := dbq.New(d.Pool)
		rows, err := q.ListNotifications(r.Context(), params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		hasMore := len(rows) > limit
		if hasMore {
			rows = rows[:limit]
		}
		// Hydrate tasks + actors. Collect ids first to batch.
		taskIDs := map[uuid.UUID]struct{}{}
		actorIDs := map[uuid.UUID]struct{}{}
		for _, row := range rows {
			if row.TaskID != nil {
				taskIDs[*row.TaskID] = struct{}{}
			}
			if row.ActorUserID != nil {
				actorIDs[*row.ActorUserID] = struct{}{}
			}
		}
		tasksByID := map[uuid.UUID]map[string]any{}
		for id := range taskIDs {
			if t, err := tasks.Get(r.Context(), d.Pool, id); err == nil {
				tasksByID[id] = map[string]any{
					"id":         t.ID,
					"title":      t.Title,
					"project_id": t.ProjectID,
				}
			}
		}
		actorsByID := map[uuid.UUID]map[string]any{}
		for id := range actorIDs {
			if u, err := identity.GetUser(r.Context(), d.Pool, id); err == nil {
				actorsByID[id] = map[string]any{
					"user_id":      u.ID,
					"github_login": u.GithubLogin,
					"display_name": u.DisplayName,
					"avatar_url":   u.AvatarURL,
				}
			}
		}
		out := make([]map[string]any, 0, len(rows))
		var nextAfterCreated *time.Time
		var nextAfterID *uuid.UUID
		for i, row := range rows {
			var readAt any
			if row.ReadAt.Valid {
				readAt = row.ReadAt.Time
			}
			item := map[string]any{
				"id":         row.ID,
				"kind":       row.Kind,
				"body":       row.Body,
				"created_at": row.CreatedAt.Time,
				"read_at":    readAt,
			}
			if row.TaskID != nil {
				if hydrated, ok := tasksByID[*row.TaskID]; ok {
					item["task"] = hydrated
				}
			}
			if row.ActorUserID != nil {
				if hydrated, ok := actorsByID[*row.ActorUserID]; ok {
					item["actor"] = hydrated
				}
			}
			out = append(out, item)
			if hasMore && i == len(rows)-1 {
				t := row.CreatedAt.Time
				id := row.ID
				nextAfterCreated = &t
				nextAfterID = &id
			}
		}
		resp := map[string]any{
			"notifications": out,
			"next_after":    nil,
		}
		if nextAfterCreated != nil {
			resp["next_after"] = map[string]any{
				"created_at": nextAfterCreated,
				"id":         nextAfterID,
			}
		}
		writeJSON(w, http.StatusOK, resp)
	})
}

// UnreadCountHandler returns the caller's unread notification count.
// Cheap thanks to the partial index over rows with NULL read_at.
func UnreadCountHandler(d NotificationsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if !d.Enabled {
			writeJSON(w, http.StatusOK, map[string]any{"unread": 0, "disabled": true})
			return
		}
		n, err := dbq.New(d.Pool).CountUnread(r.Context(), c.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"unread": n})
	})
}

type markReadReq struct {
	IDs []uuid.UUID `json:"ids"`
}

// MarkReadHandler marks the given ids as read for the caller only.
// Ids belonging to other users are silently ignored (the SQL WHERE
// clause scopes on user_id). Response reports how many rows moved
// from unread to read.
func MarkReadHandler(d NotificationsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if !d.Enabled {
			http.Error(w, "notifications disabled", http.StatusNotImplemented)
			return
		}
		var req markReadReq
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if len(req.IDs) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"marked": 0})
			return
		}
		n, err := dbq.New(d.Pool).MarkRead(r.Context(), dbq.MarkReadParams{
			UserID: c.UserID,
			Ids:    req.IDs,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"marked": n})
	})
}

// MarkAllReadHandler marks every unread notification for the caller
// as read.
func MarkAllReadHandler(d NotificationsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if !d.Enabled {
			http.Error(w, "notifications disabled", http.StatusNotImplemented)
			return
		}
		n, err := dbq.New(d.Pool).MarkAllRead(r.Context(), c.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"marked": n})
	})
}

// defaultPrefs returns the shape the frontend expects: every allowed
// key present with its default (true). Merged with the stored row so
// a new user (empty JSONB) still sees the full block.
func defaultPrefs() map[string]bool {
	out := map[string]bool{}
	for k := range AllowedPrefKeys {
		out[k] = true
	}
	return out
}

// GetPreferencesHandler returns the caller's per-kind preference
// toggles, defaults filled in.
func GetPreferencesHandler(d NotificationsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		raw, err := dbq.New(d.Pool).GetPreferences(r.Context(), c.UserID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		prefs := defaultPrefs()
		if len(raw) > 0 {
			m := map[string]bool{}
			if err := json.Unmarshal(raw, &m); err == nil {
				for k, v := range m {
					if _, allowed := AllowedPrefKeys[k]; allowed {
						prefs[k] = v
					}
				}
			}
		}
		writeJSON(w, http.StatusOK, prefs)
	})
}

// PatchPreferencesHandler merges the request body into the caller's
// prefs. Unknown keys are 400. Values must be booleans (JSON true/
// false); anything else is 400.
func PatchPreferencesHandler(d NotificationsDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.caller(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		var incoming map[string]bool
		if err := decodeJSON(r, &incoming); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		for k := range incoming {
			if _, allowed := AllowedPrefKeys[k]; !allowed {
				writeError(w, http.StatusBadRequest, "unknown preference key: "+k)
				return
			}
		}
		q := dbq.New(d.Pool)
		raw, err := q.GetPreferences(r.Context(), c.UserID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		merged := defaultPrefs()
		if len(raw) > 0 {
			m := map[string]bool{}
			if err := json.Unmarshal(raw, &m); err == nil {
				for k, v := range m {
					if _, allowed := AllowedPrefKeys[k]; allowed {
						merged[k] = v
					}
				}
			}
		}
		for k, v := range incoming {
			merged[k] = v
		}
		blob, err := json.Marshal(merged)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := q.UpdatePreferences(r.Context(), dbq.UpdatePreferencesParams{
			UserID: c.UserID,
			Prefs:  blob,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, merged)
	})
}
