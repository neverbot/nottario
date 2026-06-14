package identity

import (
	"context"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/db/dbq"
)

// ViaMCP marks a row as having been written through an MCP token (an
// agent acting on behalf of a human) rather than directly through the
// web UI. Name is the human-readable token name set at issuance — what
// the user typed when generating the token, e.g. "claude-cli-laptop".
//
// We deliberately do NOT expose the token's UUID or any other identifying
// data: the front-end only needs to know "this was an agent, and which
// one of my agents" to render the right badge / suffix, and the name
// is set by the human who issued the token (so they control what shows
// up). The UUID stays server-side.
type ViaMCP struct {
	Name string `json:"name"`
}

// LookupTokenNames returns a map from token id to token name for the
// given non-nil ids. Token ids that are not found are omitted from the
// map — the caller should treat their absence as "row recorded via a
// token that no longer exists" (e.g. project deleted, ancient row) and
// fall back to a generic agent marker.
//
// Pass a `dbq.DBTX` (the same handle the caller is already using —
// either a *pgxpool.Pool or a pgx.Tx). Returning an empty map for an
// empty input avoids a wasted round-trip.
func LookupTokenNames(ctx context.Context, db dbq.DBTX, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]string{}, nil
	}
	rows, err := dbq.New(db).ListTokenNamesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out, nil
}

// ViaMCPFromMap is a tiny helper for repo code: given the optional
// token id pointer recorded on a row and the lookup map returned by
// LookupTokenNames, returns the ViaMCP that should be set on the
// outgoing struct (nil when the row is a direct-human action OR when
// the token has since disappeared).
func ViaMCPFromMap(tokenID *uuid.UUID, names map[uuid.UUID]string) *ViaMCP {
	if tokenID == nil {
		return nil
	}
	name, ok := names[*tokenID]
	if !ok {
		// Token row gone — surface "agent" without a name. Anonymous
		// agent badge in the UI.
		return &ViaMCP{Name: ""}
	}
	return &ViaMCP{Name: name}
}
