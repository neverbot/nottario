package identity

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// DefaultRoleCatalogue is seeded into every new project.
var DefaultRoleCatalogue = []Role{
	{Key: "backend", Label: "Backend", Color: "#1f6feb"},
	{Key: "frontend", Label: "Frontend", Color: "#2da44e"},
	{Key: "qa", Label: "QA", Color: "#bf8700"},
	{Key: "design", Label: "Design", Color: "#a371f7"},
}

// CreateProject persists a new project with a generated slug, seeds
// the default role catalogue and attaches the initial repo list.
func CreateProject(ctx context.Context, pool *pgxpool.Pool, name, description, primaryLanguage, projectType string, createdByUserID uuid.UUID, repos []string) (*Project, error) {
	slug, err := uniqueSlug(ctx, pool, name)
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)

	row, err := q.InsertProject(ctx, dbq.InsertProjectParams{
		Slug:            slug,
		Name:            name,
		Description:     description,
		PrimaryLanguage: primaryLanguage,
		ProjectType:     projectType,
		CreatedByUserID: &createdByUserID,
		OwnerUserID:     createdByUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("insert project: %w", err)
	}
	p := projectFromInsertRow(row)

	for i, r := range DefaultRoleCatalogue {
		roleID, err := q.InsertSeedRole(ctx, dbq.InsertSeedRoleParams{
			ProjectID: p.ID,
			Key:       r.Key,
			Label:     r.Label,
			Color:     pgtype.Text{String: r.Color, Valid: r.Color != ""},
			Position:  int32(i),
		})
		if err != nil {
			return nil, fmt.Errorf("seed role %s: %w", r.Key, err)
		}
		if err := q.InsertMembership(ctx, dbq.InsertMembershipParams{
			UserID:    createdByUserID,
			ProjectID: p.ID,
			RoleID:    roleID,
		}); err != nil {
			return nil, fmt.Errorf("auto-join role %s: %w", r.Key, err)
		}
	}
	if err := seedDefaultPriorities(ctx, tx, p.ID); err != nil {
		return nil, err
	}
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		if err := q.InsertProjectRepo(ctx, dbq.InsertProjectRepoParams{
			ProjectID: p.ID,
			Repo:      repo,
		}); err != nil {
			return nil, fmt.Errorf("attach repo %s: %w", repo, err)
		}
		p.Repos = append(p.Repos, repo)
	}
	if _, err := q.InsertCycle(ctx, dbq.InsertCycleParams{
		ProjectID: p.ID,
		Name:      "sprint-1",
		Position:  1,
	}); err != nil {
		return nil, fmt.Errorf("seed initial cycle: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &p, nil
}

// ListProjects returns projects visible to the caller. Admins see
// every project; others see only the projects where they have a
// membership.
func ListProjects(ctx context.Context, pool *pgxpool.Pool, callerUserID uuid.UUID, isAdmin bool) ([]Project, error) {
	q := dbq.New(pool)
	var out []Project
	if isAdmin {
		rows, err := q.ListProjectsAdmin(ctx)
		if err != nil {
			return nil, err
		}
		out = make([]Project, 0, len(rows))
		for _, r := range rows {
			out = append(out, Project{
				ID:              r.ID,
				Slug:            r.Slug,
				Name:            r.Name,
				Description:     r.Description,
				PrimaryLanguage: r.PrimaryLanguage,
				ProjectType:     r.ProjectType,
				MCPPageSize:     int(r.McpPageSize),
				DefaultView:     r.DefaultView,
				CycleLabel:      r.CycleLabel,
				OwnerUserID:     r.OwnerUserID,
				CreatedByUserID: r.CreatedByUserID,
				CreatedAt:       r.CreatedAt.Time,
				UpdatedAt:       r.UpdatedAt.Time,
			})
		}
	} else {
		rows, err := q.ListProjectsForUser(ctx, callerUserID)
		if err != nil {
			return nil, err
		}
		out = make([]Project, 0, len(rows))
		for _, r := range rows {
			out = append(out, Project{
				ID:              r.ID,
				Slug:            r.Slug,
				Name:            r.Name,
				Description:     r.Description,
				PrimaryLanguage: r.PrimaryLanguage,
				ProjectType:     r.ProjectType,
				MCPPageSize:     int(r.McpPageSize),
				DefaultView:     r.DefaultView,
				CycleLabel:      r.CycleLabel,
				OwnerUserID:     r.OwnerUserID,
				CreatedByUserID: r.CreatedByUserID,
				CreatedAt:       r.CreatedAt.Time,
				UpdatedAt:       r.UpdatedAt.Time,
			})
		}
	}
	for i := range out {
		repos, err := q.ListProjectRepos(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Repos = repos
	}
	if err := enrichProjects(ctx, q, out); err != nil {
		return nil, err
	}
	return out, nil
}

// enrichProjects attaches Stats and Members to every project in the
// slice using two batch queries. Best-effort: a failure attaching
// stats does not blank the whole list (the cards degrade to
// language/type/repos only).
func enrichProjects(ctx context.Context, q *dbq.Queries, projects []Project) error {
	if len(projects) == 0 {
		return nil
	}
	statsByID := map[uuid.UUID]*ProjectStats{}
	if rows, err := q.ListAllProjectTaskStats(ctx); err == nil {
		for _, r := range rows {
			s := &ProjectStats{
				TodoCount:  int(r.TodoCount),
				DoingCount: int(r.DoingCount),
				DoneCount:  int(r.DoneCount),
			}
			if r.LastActivityAt.Valid {
				v := r.LastActivityAt.Time
				s.LastActivityAt = &v
			}
			statsByID[r.ProjectID] = s
		}
	}
	membersByID := map[uuid.UUID][]ProjectMember{}
	if rows, err := q.ListAllProjectMembers(ctx); err == nil {
		for _, r := range rows {
			membersByID[r.ProjectID] = append(membersByID[r.ProjectID], ProjectMember{
				UserID:      r.UserID,
				GithubLogin: r.GithubLogin,
				DisplayName: r.DisplayName,
				AvatarURL:   r.AvatarUrl,
			})
		}
	}
	for i := range projects {
		if s, ok := statsByID[projects[i].ID]; ok {
			projects[i].Stats = s
		} else {
			projects[i].Stats = &ProjectStats{}
		}
		projects[i].Members = membersByID[projects[i].ID]
	}
	return nil
}

// GetProject loads a single project by uuid or slug.
func GetProject(ctx context.Context, pool *pgxpool.Pool, idOrSlug string) (*Project, error) {
	q := dbq.New(pool)
	row, err := q.GetProjectByIDOrSlug(ctx, idOrSlug)
	if err != nil {
		return nil, err
	}
	p := Project{
		ID:              row.ID,
		Slug:            row.Slug,
		Name:            row.Name,
		Description:     row.Description,
		PrimaryLanguage: row.PrimaryLanguage,
		ProjectType:     row.ProjectType,
		MCPPageSize:     int(row.McpPageSize),
		DefaultView:     row.DefaultView,
		CycleLabel:      row.CycleLabel,
		OwnerUserID:     row.OwnerUserID,
		CreatedByUserID: row.CreatedByUserID,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}
	repos, err := q.ListProjectRepos(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Repos = repos
	return &p, nil
}

// UpdateProject mutates the human-editable fields. defaultView is
// optional: when empty, the column is left untouched; otherwise it
// must be a key from ValidDefaultViews.
func UpdateProject(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, name, description, primaryLanguage, projectType, defaultView string, repos []string) (*Project, error) {
	if defaultView != "" && !ValidDefaultViews[defaultView] {
		return nil, errors.New("invalid default_view")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)

	if err := q.UpdateProjectFields(ctx, dbq.UpdateProjectFieldsParams{
		ID:              id,
		Name:            name,
		Description:     description,
		PrimaryLanguage: primaryLanguage,
		ProjectType:     projectType,
	}); err != nil {
		return nil, err
	}
	if defaultView != "" {
		if err := q.UpdateProjectDefaultView(ctx, dbq.UpdateProjectDefaultViewParams{
			ID:          id,
			DefaultView: defaultView,
		}); err != nil {
			return nil, err
		}
	}
	if repos != nil {
		if err := q.ClearProjectRepos(ctx, id); err != nil {
			return nil, err
		}
		for _, r := range repos {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			if err := q.InsertProjectRepo(ctx, dbq.InsertProjectRepoParams{
				ProjectID: id, Repo: r,
			}); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return GetProject(ctx, pool, id.String())
}

// UpdateProjectMCPPageSize sets the per-project default page size used
// by `tasks.list` over MCP when the caller doesn't pass an explicit
// `limit`. The DB CHECK constraint enforces the 1..500 range.
func UpdateProjectMCPPageSize(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, pageSize int) (*Project, error) {
	if pageSize < 1 || pageSize > 500 {
		return nil, errors.New("mcp_page_size must be between 1 and 500")
	}
	if err := dbq.New(pool).UpdateProjectMCPPageSize(ctx, dbq.UpdateProjectMCPPageSizeParams{
		ID:          id,
		McpPageSize: int32(pageSize),
	}); err != nil {
		return nil, err
	}
	return GetProject(ctx, pool, id.String())
}

// ValidDefaultViews is the allowlist of view keys mirrored from the
// frontend's view registry (internal/web/static/views.js) and enforced
// also by a CHECK constraint on the column. Keep the two in sync when
// new views are added.
var ValidDefaultViews = map[string]bool{
	"board/kanban": true,
	"board/gantt":  true,
	"docs":         true,
	"arch/diagram": true,
	"arch/tree":    true,
}

// UpdateProjectDefaultView sets the per-project landing view used by
// the project cards on `/`. Value must be one of ValidDefaultViews.
func UpdateProjectDefaultView(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, view string) (*Project, error) {
	if !ValidDefaultViews[view] {
		return nil, errors.New("invalid default_view")
	}
	if err := dbq.New(pool).UpdateProjectDefaultView(ctx, dbq.UpdateProjectDefaultViewParams{
		ID:          id,
		DefaultView: view,
	}); err != nil {
		return nil, err
	}
	return GetProject(ctx, pool, id.String())
}

// DeleteProject removes a project and cascades all dependent rows.
func DeleteProject(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	return dbq.New(pool).DeleteProjectByID(ctx, id)
}

func projectFromInsertRow(r dbq.InsertProjectRow) Project {
	return Project{
		ID:              r.ID,
		Slug:            r.Slug,
		Name:            r.Name,
		Description:     r.Description,
		PrimaryLanguage: r.PrimaryLanguage,
		ProjectType:     r.ProjectType,
		MCPPageSize:     int(r.McpPageSize),
		DefaultView:     r.DefaultView,
		CycleLabel:      r.CycleLabel,
		OwnerUserID:     r.OwnerUserID,
		CreatedByUserID: r.CreatedByUserID,
		CreatedAt:       r.CreatedAt.Time,
		UpdatedAt:       r.UpdatedAt.Time,
	}
}

var slugSafe = regexp.MustCompile(`[^a-z0-9-]+`)

// Slugify produces a URL-safe slug from a free-text name.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugSafe.ReplaceAllString(s, "")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	if s == "" {
		s = "project"
	}
	return s
}

func uniqueSlug(ctx context.Context, pool *pgxpool.Pool, name string) (string, error) {
	q := dbq.New(pool)
	base := Slugify(name)
	candidate := base
	for i := 2; i < 100; i++ {
		exists, err := q.ProjectSlugExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
	return "", errors.New("could not allocate unique project slug")
}
