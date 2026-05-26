// Package search runs a unified full-text search across the three
// content domains (tasks, documents, architecture nodes) for one
// project.
package search

import (
	"context"
	"errors"
	"html"
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
//
// `TitleHTML` and `DescriptionHTML` carry highlighted snippets safe
// for `unsafeHTML` rendering: the raw text is HTML-escaped, then the
// `ts_headline` sentinels are swapped for `<mark>` tags. Consumers
// may fall back to `Title`/`Description` when they prefer plain text.
type Hit struct {
	Kind            Kind    `json:"kind"`
	ProjectID       string  `json:"project_id"`
	Rank            float32 `json:"rank"`
	Title           string  `json:"title"`
	Description     string  `json:"description,omitempty"`
	TitleHTML       string  `json:"title_html,omitempty"`
	DescriptionHTML string  `json:"description_html,omitempty"`
	TaskID          string  `json:"task_id,omitempty"`
	DocPath         string  `json:"doc_path,omitempty"`
	DocScope        string  `json:"doc_scope,omitempty"`
	NodeSlug        string  `json:"node_slug,omitempty"`
	NodeKind        string  `json:"node_kind,omitempty"`
	TaskState       string  `json:"task_state,omitempty"`
	TaskType        string  `json:"task_type,omitempty"`
}

// highlightSentinelStart / End are the rare strings ts_headline wraps
// around matches (configured in queries/search.sql). They are escaped
// before being swapped for real <mark> tags so user content that
// happens to contain HTML stays escaped.
const (
	highlightSentinelStart = "«MARK»"
	highlightSentinelEnd   = "«/MARK»"
)

func highlightSnippet(s string) string {
	if s == "" {
		return ""
	}
	out := html.EscapeString(s)
	out = strings.ReplaceAll(out, highlightSentinelStart, "<mark>")
	out = strings.ReplaceAll(out, highlightSentinelEnd, "</mark>")
	return out
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
			Kind:            Kind(r.Kind),
			ProjectID:       r.ProjectID,
			Rank:            r.Rank,
			Title:           r.Title,
			Description:     r.Description,
			TitleHTML:       highlightSnippet(string(r.TitleHeadline)),
			DescriptionHTML: highlightSnippet(string(r.DescriptionHeadline)),
			TaskID:          r.TaskID,
			DocPath:         r.DocPath,
			DocScope:        r.DocScope,
			NodeSlug:        r.NodeSlug,
			NodeKind:        r.NodeKind,
			TaskState:       r.TaskState,
			TaskType:        r.TaskType,
		})
	}
	return out, nil
}
