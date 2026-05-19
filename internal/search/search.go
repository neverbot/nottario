// Package search runs a unified full-text search across the three
// content domains (tasks, documents, architecture nodes) for one
// project. Each result carries enough metadata for the UI to link to
// the right place; the agent uses the same interface via the
// `nottario.search` MCP tool.
package search

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Kind identifies which domain a hit comes from.
type Kind string

const (
	KindTask     Kind = "task"
	KindDocument Kind = "document"
	KindArchNode Kind = "arch_node"
)

// Hit is one search result. Fields are intentionally flat so JSON
// consumers can branch on Kind.
type Hit struct {
	Kind       Kind    `json:"kind"`
	ProjectID  string  `json:"project_id"`
	Rank       float32 `json:"rank"`
	// Common display fields:
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	// Kind-specific identifiers (one is set, others are empty):
	TaskID      string `json:"task_id,omitempty"`
	DocPath     string `json:"doc_path,omitempty"`
	DocScope    string `json:"doc_scope,omitempty"`
	NodeSlug    string `json:"node_slug,omitempty"`
	NodeKind    string `json:"node_kind,omitempty"`
	TaskState   string `json:"task_state,omitempty"`
	TaskType    string `json:"task_type,omitempty"`
}

// Filter narrows a Search call.
type Filter struct {
	ProjectID uuid.UUID // required; the search is project-scoped
	Kinds     []Kind    // empty = all
	Limit     int       // max results overall; defaults to 50
}

// Search runs the unified query.
func Search(ctx context.Context, pool *pgxpool.Pool, query string, f Filter) ([]Hit, error) {
	if f.ProjectID == uuid.Nil {
		return nil, errors.New("project_id is required")
	}
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("query is required")
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	include := func(k Kind) bool {
		if len(f.Kinds) == 0 {
			return true
		}
		for _, x := range f.Kinds {
			if x == k {
				return true
			}
		}
		return false
	}

	var parts []string
	args := []any{f.ProjectID, query}

	if include(KindTask) {
		parts = append(parts, `
			SELECT 'task'::text AS kind,
			       project_id::text AS project_id,
			       ts_rank(search_vector, plainto_tsquery('simple', $2)) AS rank,
			       title,
			       left(coalesce(description_md, ''), 200) AS description,
			       id::text AS task_id,
			       ''::text AS doc_path, ''::text AS doc_scope,
			       ''::text AS node_slug, ''::text AS node_kind,
			       state AS task_state, type AS task_type
			FROM tasks
			WHERE project_id = $1 AND search_vector @@ plainto_tsquery('simple', $2)
		`)
	}
	if include(KindDocument) {
		parts = append(parts, `
			SELECT 'document'::text AS kind,
			       coalesce(project_id::text, '') AS project_id,
			       ts_rank(search_vector, plainto_tsquery('simple', $2)) AS rank,
			       title,
			       left(coalesce(description, ''), 200) AS description,
			       ''::text AS task_id,
			       path AS doc_path, scope AS doc_scope,
			       ''::text AS node_slug, ''::text AS node_kind,
			       ''::text AS task_state, ''::text AS task_type
			FROM documents
			WHERE (project_id = $1 OR scope = 'global')
			  AND deleted_at IS NULL
			  AND search_vector @@ plainto_tsquery('simple', $2)
		`)
	}
	if include(KindArchNode) {
		parts = append(parts, `
			SELECT 'arch_node'::text AS kind,
			       project_id::text AS project_id,
			       ts_rank(search_vector, plainto_tsquery('simple', $2)) AS rank,
			       name AS title,
			       left(coalesce(description_md, ''), 200) AS description,
			       ''::text AS task_id,
			       ''::text AS doc_path, ''::text AS doc_scope,
			       slug AS node_slug, kind AS node_kind,
			       ''::text AS task_state, ''::text AS task_type
			FROM arch_nodes
			WHERE project_id = $1 AND search_vector @@ plainto_tsquery('simple', $2)
		`)
	}
	if len(parts) == 0 {
		return []Hit{}, nil
	}

	sql := strings.Join(parts, "\nUNION ALL\n") +
		fmt.Sprintf("\nORDER BY rank DESC LIMIT $%d", len(args)+1)
	args = append(args, limit)

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hit{}
	for rows.Next() {
		var h Hit
		if err := rows.Scan(
			&h.Kind, &h.ProjectID, &h.Rank,
			&h.Title, &h.Description,
			&h.TaskID, &h.DocPath, &h.DocScope,
			&h.NodeSlug, &h.NodeKind,
			&h.TaskState, &h.TaskType,
		); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
