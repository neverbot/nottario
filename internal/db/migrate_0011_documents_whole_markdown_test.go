package db_test

import (
	"context"
	"testing"

	"github.com/neverbot/nottario/internal/db"
	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/testutil"
)

// Documents written before 0011 have their frontmatter split off. The
// migration must rebuild them whole, leave frontmatter-less rows alone,
// keep search off the frontmatter, and be reversible.
func TestMigration0011_DocumentsWholeMarkdown(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := context.Background()

	if err := db.MigrateTo(ctx, pool, 9); err != nil {
		t.Fatalf("migrate down to 9: %v", err)
	}

	const legacyBody = "# Plan\n\nThe body mentions ñandú.\n"
	const legacyFM = `{"title": "Plan", "kind": "context", "description": "a: b", "tags": ["zebra", "yak"], "n": 3}`
	const plain = "# Plain\n\nNo frontmatter.\n"

	var withID, plainID string
	if err := pool.QueryRow(ctx, `INSERT INTO documents (scope, path, kind, title, description, content_md, frontmatter)
		VALUES ('global', 'plan.md', 'context', 'Plan', 'a: b', $1, $2::jsonb) RETURNING id::text`,
		legacyBody, legacyFM).Scan(&withID); err != nil {
		t.Fatalf("insert legacy: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO document_versions (document_id, version, title, description, content_md, frontmatter)
		VALUES ($1::uuid, 1, 'Plan', 'a: b', $2, $3::jsonb)`, withID, legacyBody, legacyFM); err != nil {
		t.Fatalf("insert legacy version: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO documents (scope, path, kind, title, content_md)
		VALUES ('global', 'plain.md', 'context', 'Plain', $1) RETURNING id::text`, plain).Scan(&plainID); err != nil {
		t.Fatalf("insert plain: %v", err)
	}

	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	var content string
	var offset int
	if err := pool.QueryRow(ctx, `SELECT content_md, body_offset FROM documents WHERE id = $1::uuid`, withID).Scan(&content, &offset); err != nil {
		t.Fatal(err)
	}
	fm, body, err := docs.SplitFrontmatter(content)
	if err != nil {
		t.Fatalf("migrated document does not parse: %v\n%s", err, content)
	}
	if body != legacyBody {
		t.Errorf("body changed:\n got %q\nwant %q", body, legacyBody)
	}
	if docs.TitleFromFrontmatter(fm) != "Plan" || docs.DescriptionFromFrontmatter(fm) != "a: b" || fm["n"] != 3 {
		t.Errorf("frontmatter not rebuilt faithfully: %#v", fm)
	}
	if offset != docs.BodyOffset(content) || offset == 0 {
		t.Errorf("body_offset = %d, want %d", offset, docs.BodyOffset(content))
	}

	var versionContent string
	if err := pool.QueryRow(ctx, `SELECT content_md FROM document_versions WHERE document_id = $1::uuid`, withID).Scan(&versionContent); err != nil {
		t.Fatal(err)
	}
	if versionContent != content {
		t.Errorf("version not rebuilt like the document:\n got %q\nwant %q", versionContent, content)
	}

	var plainContent string
	var plainOffset int
	if err := pool.QueryRow(ctx, `SELECT content_md, body_offset FROM documents WHERE id = $1::uuid`, plainID).Scan(&plainContent, &plainOffset); err != nil {
		t.Fatal(err)
	}
	if plainContent != plain || plainOffset != 0 {
		t.Errorf("document without frontmatter was touched: %q offset %d", plainContent, plainOffset)
	}

	// "zebra" only exists inside the frontmatter; "ñandú" only in the body.
	for word, want := range map[string]bool{"zebra": false, "ñandú": true} {
		var hit bool
		if err := pool.QueryRow(ctx, `SELECT search_vector @@ plainto_tsquery('simple', $1) FROM documents WHERE id = $2::uuid`, word, withID).Scan(&hit); err != nil {
			t.Fatal(err)
		}
		if hit != want {
			t.Errorf("search for %q matched=%v, want %v", word, hit, want)
		}
	}

	if err := db.MigrateTo(ctx, pool, 9); err != nil {
		t.Fatalf("migrate down again: %v", err)
	}
	var reverted string
	if err := pool.QueryRow(ctx, `SELECT content_md FROM documents WHERE id = $1::uuid`, withID).Scan(&reverted); err != nil {
		t.Fatal(err)
	}
	if reverted != legacyBody {
		t.Errorf("down did not restore the split body: %q", reverted)
	}
}
