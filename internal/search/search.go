// Package search runs a unified full-text search across the three
// content domains (tasks, documents, architecture nodes) for one
// project.
package search

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// Kind identifies which domain a hit comes from.
type Kind string

const (
	KindTask     Kind = "task"
	KindDocument Kind = "document"
	KindArchNode Kind = "arch_node"
)

// Hit is one search result.
type Hit struct {
	Kind        Kind    `json:"kind"`
	ProjectID   string  `json:"project_id"`
	Rank        float32 `json:"rank"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	TaskID      string  `json:"task_id,omitempty"`
	DocPath     string  `json:"doc_path,omitempty"`
	DocScope    string  `json:"doc_scope,omitempty"`
	NodeSlug    string  `json:"node_slug,omitempty"`
	NodeKind    string  `json:"node_kind,omitempty"`
	TaskState   string  `json:"task_state,omitempty"`
	TaskType    string  `json:"task_type,omitempty"`
}

// Filter narrows a Search call.
type Filter struct {
	ProjectID uuid.UUID
	Kinds     []Kind
	Limit     int
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

	rows, err := dbq.New(pool).UnifiedSearch(ctx, dbq.UnifiedSearchParams{
		Lim:             int32(limit),
		Query:           query,
		ProjectID:       f.ProjectID,
		IncludeTask:     include(KindTask),
		IncludeDocument: include(KindDocument),
		IncludeArchNode: include(KindArchNode),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Hit, 0, len(rows))
	for _, r := range rows {
		out = append(out, Hit{
			Kind:        Kind(r.Kind),
			ProjectID:   r.ProjectID,
			Rank:        r.Rank,
			Title:       r.Title,
			Description: r.Description,
			TaskID:      r.TaskID,
			DocPath:     r.DocPath,
			DocScope:    r.DocScope,
			NodeSlug:    r.NodeSlug,
			NodeKind:    r.NodeKind,
			TaskState:   r.TaskState,
			TaskType:    r.TaskType,
		})
	}
	return out, nil
}
