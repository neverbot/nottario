package tasks

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// CloseCommit is one optional commit link to attach during Close.
type CloseCommit struct {
	Repo    string
	SHA     string
	Message string
}

// CloseParams carries the optional add_comment + link_commit[*]
// payload that piggy-backs on the state transition. Comment is
// applied first, then each commit link, then the state transition —
// all inside a single Postgres transaction. Validation happens up
// front so a malformed input never produces a partial write.
type CloseParams struct {
	State   State
	Comment string
	Commits []CloseCommit
}

// CloseResult is what Close returns on success.
type CloseResult struct {
	Task         *Task
	CommentID    *uuid.UUID
	LinkedCommit int
}

// Close atomically attaches commit links + an optional comment and
// transitions the task to the requested terminal state. All three
// operations share one transaction, so a failed precondition rolls
// back the comment and links too — the caller's request reads as
// "nothing happened" instead of "comment and links landed but the
// state didn't move".
//
// All inputs except State are optional; passing only State degrades
// to a plain SetState (same shape, same checks). The caller decides
// whether to gate Close on `wont_do` vs `done` semantics — this layer
// runs whatever the State enum accepts.
func Close(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID, p CloseParams, by Authorship) (*CloseResult, error) {
	// Up-front validation so we never start a transaction that we know
	// will fail mid-way. setStateTx validates State on its own, but the
	// comment body and commit shape are ours to check.
	body := strings.TrimSpace(p.Comment)
	type commit struct{ repo, sha, msg string }
	normalised := make([]commit, 0, len(p.Commits))
	for _, c := range p.Commits {
		repo := strings.TrimSpace(c.Repo)
		sha := strings.TrimSpace(c.SHA)
		if repo == "" || sha == "" {
			return nil, errInvalid("each commit must have repo and sha")
		}
		normalised = append(normalised, commit{repo: repo, sha: sha, msg: c.Message})
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)

	var commentID *uuid.UUID
	if body != "" {
		row, err := q.InsertTaskComment(ctx, dbq.InsertTaskCommentParams{
			TaskID:        taskID,
			AuthorUserID:  by.UserID,
			AuthorTokenID: by.TokenID,
			BodyMd:        body,
		})
		if err != nil {
			return nil, err
		}
		id := row.ID
		commentID = &id
	}

	for _, c := range normalised {
		if err := q.UpsertTaskCommit(ctx, dbq.UpsertTaskCommitParams{
			TaskID:         taskID,
			Repo:           c.repo,
			Sha:            c.sha,
			Message:        c.msg,
			AddedByUserID:  by.UserID,
			AddedByTokenID: by.TokenID,
		}); err != nil {
			return nil, err
		}
	}

	if err := setStateTx(ctx, tx, taskID, p.State); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	t, err := Get(ctx, pool, taskID)
	if err != nil {
		return nil, err
	}
	return &CloseResult{
		Task:         t,
		CommentID:    commentID,
		LinkedCommit: len(normalised),
	}, nil
}
