package tasks

import (
	"context"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/identity"
)

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
