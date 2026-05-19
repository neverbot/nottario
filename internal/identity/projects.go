package identity

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	defer tx.Rollback(ctx)

	var p Project
	err = tx.QueryRow(ctx, `
		INSERT INTO projects (slug, name, description, primary_language, project_type, created_by_user_id)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6)
		RETURNING id, slug, name, description,
		          COALESCE(primary_language, ''), COALESCE(project_type, ''),
		          created_by_user_id, created_at, updated_at
	`, slug, name, description, primaryLanguage, projectType, createdByUserID).Scan(
		&p.ID, &p.Slug, &p.Name, &p.Description,
		&p.PrimaryLanguage, &p.ProjectType,
		&p.CreatedByUserID, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert project: %w", err)
	}

	for _, r := range DefaultRoleCatalogue {
		_, err = tx.Exec(ctx, `
			INSERT INTO roles (project_id, key, label, color)
			VALUES ($1, $2, $3, $4)
		`, p.ID, r.Key, r.Label, r.Color)
		if err != nil {
			return nil, fmt.Errorf("seed role %s: %w", r.Key, err)
		}
	}

	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO project_repos (project_id, repo)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, p.ID, repo)
		if err != nil {
			return nil, fmt.Errorf("attach repo %s: %w", repo, err)
		}
		p.Repos = append(p.Repos, repo)
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
	var rows pgx.Rows
	var err error
	if isAdmin {
		rows, err = pool.Query(ctx, `
			SELECT id, slug, name, description,
			       COALESCE(primary_language, ''), COALESCE(project_type, ''),
			       created_by_user_id, created_at, updated_at
			FROM projects ORDER BY name
		`)
	} else {
		rows, err = pool.Query(ctx, `
			SELECT DISTINCT p.id, p.slug, p.name, p.description,
			       COALESCE(p.primary_language, ''), COALESCE(p.project_type, ''),
			       p.created_by_user_id, p.created_at, p.updated_at
			FROM projects p
			JOIN memberships m ON m.project_id = p.id
			WHERE m.user_id = $1
			ORDER BY p.name
		`, callerUserID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(
			&p.ID, &p.Slug, &p.Name, &p.Description,
			&p.PrimaryLanguage, &p.ProjectType,
			&p.CreatedByUserID, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		repos, err := listProjectRepos(ctx, pool, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Repos = repos
	}
	return out, nil
}

// GetProject loads a single project by uuid or slug.
func GetProject(ctx context.Context, pool *pgxpool.Pool, idOrSlug string) (*Project, error) {
	var p Project
	row := pool.QueryRow(ctx, `
		SELECT id, slug, name, description,
		       COALESCE(primary_language, ''), COALESCE(project_type, ''),
		       created_by_user_id, created_at, updated_at
		FROM projects
		WHERE id::text = $1 OR slug = $1
	`, idOrSlug)
	if err := row.Scan(
		&p.ID, &p.Slug, &p.Name, &p.Description,
		&p.PrimaryLanguage, &p.ProjectType,
		&p.CreatedByUserID, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	repos, err := listProjectRepos(ctx, pool, p.ID)
	if err != nil {
		return nil, err
	}
	p.Repos = repos
	return &p, nil
}

// UpdateProject mutates the human-editable fields.
func UpdateProject(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, name, description, primaryLanguage, projectType string, repos []string) (*Project, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE projects
		SET name = $2,
		    description = $3,
		    primary_language = NULLIF($4, ''),
		    project_type = NULLIF($5, ''),
		    updated_at = now()
		WHERE id = $1
	`, id, name, description, primaryLanguage, projectType)
	if err != nil {
		return nil, err
	}

	if repos != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM project_repos WHERE project_id = $1`, id); err != nil {
			return nil, err
		}
		for _, r := range repos {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO project_repos (project_id, repo) VALUES ($1, $2)
				ON CONFLICT DO NOTHING
			`, id, r); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return GetProject(ctx, pool, id.String())
}

// DeleteProject removes a project and cascades all dependent rows.
func DeleteProject(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) error {
	_, err := pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	return err
}

func listProjectRepos(ctx context.Context, pool *pgxpool.Pool, projectID uuid.UUID) ([]string, error) {
	rows, err := pool.Query(ctx, `SELECT repo FROM project_repos WHERE project_id = $1 ORDER BY repo`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
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
	base := Slugify(name)
	candidate := base
	for i := 2; i < 100; i++ {
		var exists bool
		err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM projects WHERE slug = $1)`, candidate).Scan(&exists)
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
