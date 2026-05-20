package tasks

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// LinkCommit attaches a git commit to a task. Repo must be in
// "owner/repo" format; sha is any non-empty string (full or short).
func LinkCommit(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID, repo, sha, message string, by Authorship) error {
	repo = strings.TrimSpace(repo)
	sha = strings.TrimSpace(sha)
	if repo == "" || sha == "" {
		return errInvalid("repo and sha are required")
	}
	return dbq.New(pool).UpsertTaskCommit(ctx, dbq.UpsertTaskCommitParams{
		TaskID:         taskID,
		Repo:           repo,
		Sha:            sha,
		Message:        message,
		AddedByUserID:  by.UserID,
		AddedByTokenID: by.TokenID,
	})
}

// UnlinkCommit removes the (repo, sha) link from the task.
func UnlinkCommit(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID, repo, sha string) error {
	return dbq.New(pool).DeleteTaskCommit(ctx, dbq.DeleteTaskCommitParams{
		TaskID: taskID, Repo: repo, Sha: sha,
	})
}

// ListCommits returns the commits attached to a task.
func ListCommits(ctx context.Context, pool *pgxpool.Pool, taskID uuid.UUID) ([]CommitLink, error) {
	rows, err := dbq.New(pool).ListTaskCommits(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]CommitLink, 0, len(rows))
	for _, r := range rows {
		out = append(out, CommitLink{
			TaskID:  r.TaskID,
			Repo:    r.Repo,
			SHA:     r.Sha,
			Message: r.Message,
			AddedAt: r.AddedAt.Time,
		})
	}
	return out, nil
}

type invalidErr string

func (e invalidErr) Error() string { return string(e) }

func errInvalid(msg string) error { return invalidErr(msg) }
