package tasks

import (
	"context"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/identity"
)

// enrichTasks fills the derived fields every task read returns: the
// token behind an MCP-created task, the step progress and the open
// question count. All are batched across the slice.
func enrichTasks(ctx context.Context, db dbq.DBTX, tasks []*Task) error {
	if err := enrichTaskViaMCP(ctx, db, tasks); err != nil {
		return err
	}
	if err := enrichTaskStepProgress(ctx, db, tasks); err != nil {
		return err
	}
	return enrichTaskQuestionsOpen(ctx, db, tasks)
}

// enrichTaskQuestionsOpen fills QuestionsOpen on every task, in one
// query for the whole slice.
func enrichTaskQuestionsOpen(ctx context.Context, db dbq.DBTX, tasks []*Task) error {
	if len(tasks) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	rows, err := dbq.New(db).CountOpenTaskQuestions(ctx, ids)
	if err != nil {
		return err
	}
	byID := make(map[uuid.UUID]int, len(rows))
	for _, r := range rows {
		byID[r.TaskID] = int(r.Open)
	}
	for _, t := range tasks {
		t.QuestionsOpen = byID[t.ID]
	}
	return nil
}

// enrichTaskViaMCP fills the ViaMCP field of each task whose
// CreatedByTokenID is non-nil. Tasks with no token id keep ViaMCP =
// nil (direct-human action) and are not affected.
//
// One round-trip per call: the helper batches the token-id lookup
// across the whole slice, so callers should pass the full result set
// they intend to return rather than calling this once per row.
func enrichTaskViaMCP(ctx context.Context, db dbq.DBTX, tasks []*Task) error {
	ids := make([]uuid.UUID, 0, len(tasks))
	for _, t := range tasks {
		if t.CreatedByTokenID != nil {
			ids = append(ids, *t.CreatedByTokenID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	names, err := identity.LookupTokenNames(ctx, db, ids)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		t.ViaMCP = identity.ViaMCPFromMap(t.CreatedByTokenID, names)
	}
	return nil
}
