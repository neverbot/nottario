package tasks

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// TaskSummary is a related task as the task detail lists it: enough to
// label a chip and link to it, nothing more.
type TaskSummary struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
	State State     `json:"state"`
	Type  Type      `json:"type"`
}

// Related is how one task connects to the others: the feature it
// belongs to, its own subtasks, what it waits for and what waits for
// it. Lists are never nil, so they encode as [] rather than null.
type Related struct {
	Parent    *TaskSummary  `json:"parent"`
	Children  []TaskSummary `json:"children"`
	DependsOn []TaskSummary `json:"depends_on"`
	Blocks    []TaskSummary `json:"blocks"`
}

// ListRelated returns the tasks related to t. Every relation is
// confined to one project when it is written (parent and dependency
// checks in Create and AddDependency), so no project filter is needed
// on the way out.
func ListRelated(ctx context.Context, db dbq.DBTX, t *Task) (Related, error) {
	q := dbq.New(db)
	out := Related{Children: []TaskSummary{}, DependsOn: []TaskSummary{}, Blocks: []TaskSummary{}}

	if t.ParentTaskID != nil {
		p, err := q.GetTaskSummary(ctx, *t.ParentTaskID)
		switch {
		case err == nil:
			out.Parent = &TaskSummary{ID: p.ID, Title: p.Title, State: State(p.State), Type: Type(p.Type)}
		case !errors.Is(err, pgx.ErrNoRows):
			return Related{}, err
		}
	}

	children, err := q.ListChildSummaries(ctx, &t.ID)
	if err != nil {
		return Related{}, err
	}
	for _, r := range children {
		out.Children = append(out.Children, TaskSummary{ID: r.ID, Title: r.Title, State: State(r.State), Type: Type(r.Type)})
	}

	deps, err := q.ListDependsOnSummaries(ctx, t.ID)
	if err != nil {
		return Related{}, err
	}
	for _, r := range deps {
		out.DependsOn = append(out.DependsOn, TaskSummary{ID: r.ID, Title: r.Title, State: State(r.State), Type: Type(r.Type)})
	}

	blocks, err := q.ListDependentSummaries(ctx, t.ID)
	if err != nil {
		return Related{}, err
	}
	for _, r := range blocks {
		out.Blocks = append(out.Blocks, TaskSummary{ID: r.ID, Title: r.Title, State: State(r.State), Type: Type(r.Type)})
	}
	return out, nil
}
