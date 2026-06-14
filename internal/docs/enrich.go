package docs

import (
	"context"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/identity"
)

// enrichDocViaMCP fills CreatedViaMCP / UpdatedViaMCP on each
// Document by looking up the token names referenced by the
// corresponding *ByTokenID fields. One batched round-trip per call.
func enrichDocViaMCP(ctx context.Context, db dbq.DBTX, docs []*Document) error {
	ids := make([]uuid.UUID, 0, 2*len(docs))
	for _, d := range docs {
		if d.CreatedByTokenID != nil {
			ids = append(ids, *d.CreatedByTokenID)
		}
		if d.UpdatedByTokenID != nil {
			ids = append(ids, *d.UpdatedByTokenID)
		}
	}
	names, err := identity.LookupTokenNames(ctx, db, ids)
	if err != nil {
		return err
	}
	for _, d := range docs {
		d.CreatedViaMCP = identity.ViaMCPFromMap(d.CreatedByTokenID, names)
		d.UpdatedViaMCP = identity.ViaMCPFromMap(d.UpdatedByTokenID, names)
	}
	return nil
}

// enrichSummaryViaMCP fills UpdatedViaMCP on each Summary.
func enrichSummaryViaMCP(ctx context.Context, db dbq.DBTX, items []*Summary) error {
	ids := make([]uuid.UUID, 0, len(items))
	for _, s := range items {
		if s.UpdatedByTokenID != nil {
			ids = append(ids, *s.UpdatedByTokenID)
		}
	}
	names, err := identity.LookupTokenNames(ctx, db, ids)
	if err != nil {
		return err
	}
	for _, s := range items {
		s.UpdatedViaMCP = identity.ViaMCPFromMap(s.UpdatedByTokenID, names)
	}
	return nil
}

// enrichVersionViaMCP fills ViaMCP on each Version.
func enrichVersionViaMCP(ctx context.Context, db dbq.DBTX, items []*Version) error {
	ids := make([]uuid.UUID, 0, len(items))
	for _, v := range items {
		if v.AuthorTokenID != nil {
			ids = append(ids, *v.AuthorTokenID)
		}
	}
	names, err := identity.LookupTokenNames(ctx, db, ids)
	if err != nil {
		return err
	}
	for _, v := range items {
		v.ViaMCP = identity.ViaMCPFromMap(v.AuthorTokenID, names)
	}
	return nil
}

// enrichVersionSummaryViaMCP fills ViaMCP on each VersionSummary.
func enrichVersionSummaryViaMCP(ctx context.Context, db dbq.DBTX, items []*VersionSummary) error {
	ids := make([]uuid.UUID, 0, len(items))
	for _, v := range items {
		if v.AuthorTokenID != nil {
			ids = append(ids, *v.AuthorTokenID)
		}
	}
	names, err := identity.LookupTokenNames(ctx, db, ids)
	if err != nil {
		return err
	}
	for _, v := range items {
		v.ViaMCP = identity.ViaMCPFromMap(v.AuthorTokenID, names)
	}
	return nil
}
