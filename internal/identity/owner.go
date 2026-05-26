package identity

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// ErrNotProjectOwner is returned by RequireProjectOwner when the
// caller is neither the project's owner nor an instance admin.
var ErrNotProjectOwner = errors.New("not project owner")

// RequireProjectOwner returns nil iff caller is admin OR is the
// project's owner_user_id. Used by mutation gates that today live
// on a per-project basis (close cycle, change settings, mutate
// memberships).
func RequireProjectOwner(ctx context.Context, pool *pgxpool.Pool, projectID, callerUserID uuid.UUID, isAdmin bool) error {
	if isAdmin {
		return nil
	}
	row, err := dbq.New(pool).GetProjectByIDOrSlug(ctx, projectID.String())
	if err != nil {
		return err
	}
	if row.OwnerUserID == callerUserID {
		return nil
	}
	return ErrNotProjectOwner
}

// SetProjectOwner reassigns the owner. Admin-only (caller already
// gated by the handler layer).
func SetProjectOwner(ctx context.Context, pool *pgxpool.Pool, projectID, newOwnerID uuid.UUID) error {
	return dbq.New(pool).SetProjectOwner(ctx, dbq.SetProjectOwnerParams{
		ID:          projectID,
		OwnerUserID: newOwnerID,
	})
}
