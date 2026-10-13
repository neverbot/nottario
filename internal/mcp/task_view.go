package mcp

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
)

// taskView renders tasks the way agents read them. Every byte of a tool
// result lands in an agent's context, so ids an agent would have to
// look up anyway are replaced by names it already understands: the
// target role by its key ("backend"), the assignee by "me" or a GitHub
// login, timestamps to the second in UTC. Lookups are cached for the
// life of one tool call.
type taskView struct {
	ctx    context.Context
	d      Deps
	me     uuid.UUID
	roles  map[uuid.UUID]map[uuid.UUID]string // project -> role id -> key
	logins map[uuid.UUID]string
}

func newTaskView(ctx context.Context, d Deps) *taskView {
	c, _ := callerFromContext(ctx)
	return &taskView{ctx: ctx, d: d, me: c.UserID, roles: map[uuid.UUID]map[uuid.UUID]string{}, logins: map[uuid.UUID]string{}}
}

// agentTime is a timestamp as agents get it: UTC, whole seconds.
func agentTime(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

func (v *taskView) role(projectID uuid.UUID, id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	keys, ok := v.roles[projectID]
	if !ok {
		keys = map[uuid.UUID]string{}
		if rs, err := identity.ListRoles(v.ctx, v.d.Pool, projectID); err == nil {
			for _, r := range rs {
				keys[r.ID] = r.Key
			}
		}
		v.roles[projectID] = keys
	}
	if k, ok := keys[*id]; ok {
		return k
	}
	return id.String()
}

func (v *taskView) user(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	if *id == v.me {
		return "me"
	}
	if l, ok := v.logins[*id]; ok {
		return l
	}
	l := id.String()
	if u, err := identity.GetUser(v.ctx, v.d.Pool, *id); err == nil && u.GithubLogin != "" {
		l = u.GithubLogin
	}
	v.logins[*id] = l
	return l
}

func (v *taskView) slim(t *tasks.Task) slimTask {
	return slimTask{
		ID:           t.ID,
		Type:         t.Type,
		Title:        t.Title,
		State:        t.State,
		Priority:     t.Priority,
		ParentTaskID: t.ParentTaskID,
		Role:         v.role(t.ProjectID, t.TargetRoleID),
		Assignee:     v.user(t.AssigneeUserID),
		UpdatedAt:    agentTime(t.UpdatedAt),
		Steps:        t.Steps,
	}
}

// payload returns the slim shape, or the full *tasks.Task when the
// caller asked for verbose. Every MCP mutation returns through here so
// "minimal by default" is enforced in one place.
func (v *taskView) payload(t *tasks.Task, verbose bool) any {
	if verbose {
		return t
	}
	return v.slim(t)
}

func (v *taskView) list(ts []tasks.Task, verbose bool) any {
	if verbose {
		return ts
	}
	out := make([]slimTask, len(ts))
	for i := range ts {
		out[i] = v.slim(&ts[i])
	}
	return out
}

// taskDetail is what tasks.get returns by default: the slim fields plus
// the description, which is why an agent calls get at all.
type taskDetail struct {
	slimTask
	Description string `json:"description,omitempty"`
}

func (v *taskView) detail(t *tasks.Task, verbose bool) any {
	if verbose {
		return t
	}
	return taskDetail{slimTask: v.slim(t), Description: t.DescriptionMD}
}

// resolveRoleRef reads a target role given as its key ("backend") or
// its uuid. Empty means no role. An unknown value is an error, never a
// silently dropped filter.
func resolveRoleRef(ctx context.Context, d Deps, projectID uuid.UUID, s string) (*uuid.UUID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if id, err := uuid.Parse(s); err == nil {
		return &id, nil
	}
	rs, err := identity.ListRoles(ctx, d.Pool, projectID)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rs))
	for _, r := range rs {
		if strings.EqualFold(r.Key, s) {
			id := r.ID
			return &id, nil
		}
		keys = append(keys, r.Key)
	}
	return nil, errors.New("unknown role " + s + "; this project's roles are: " + strings.Join(keys, ", "))
}

// resolveUserRef reads a user given as "me" or a uuid. Empty means none.
func resolveUserRef(ctx context.Context, s string) (*uuid.UUID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if strings.EqualFold(s, "me") {
		c, err := callerFromContext(ctx)
		if err != nil {
			return nil, err
		}
		return &c.UserID, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, errors.New("user must be 'me' or a user uuid, got " + s)
	}
	return &id, nil
}
