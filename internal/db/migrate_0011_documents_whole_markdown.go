package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	"gopkg.in/yaml.v3"

	"github.com/neverbot/nottario/internal/docs"
)

// Migration 0011 stores documents whole.
//
// Until now docs.Write split the frontmatter off: content_md held the
// body and the YAML survived only as parsed JSONB. From this version on
// content_md is the complete markdown exactly as written. The original
// bytes of existing documents are gone, so rows that had frontmatter get
// it rebuilt from the JSONB, in the same shape the skill bundle already
// served overrides in:
//
//	---
//	<yaml.Marshal of the frontmatter>
//	---
//
//	<body>
//
// Rows without frontmatter are already byte-exact and are not touched.
// Every rebuilt document is re-parsed and must yield the same body and
// frontmatter, or the migration aborts and the transaction rolls back.
//
// It is a Go migration because the rebuild needs a YAML encoder and the
// application's own frontmatter parser for body_offset; Postgres cannot
// do either.
func init() {
	goose.AddNamedMigrationContext("0011_documents_whole_markdown.go", upDocumentsWholeMarkdown, downDocumentsWholeMarkdown)
}

type splitRow struct {
	id          string
	content     string
	frontmatter []byte
}

func upDocumentsWholeMarkdown(ctx context.Context, tx *sql.Tx) error {
	docRows, err := readSplitRows(ctx, tx, `SELECT id::text, content_md, frontmatter::text FROM documents WHERE frontmatter <> '{}'::jsonb`)
	if err != nil {
		return err
	}
	for _, r := range docRows {
		whole, err := wholeMarkdown(r)
		if err != nil {
			return fmt.Errorf("document %s: %w", r.id, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE documents SET content_md = $1, body_offset = $2 WHERE id = $3::uuid`,
			whole, docs.BodyOffset(whole), r.id); err != nil {
			return fmt.Errorf("document %s: %w", r.id, err)
		}
	}

	verRows, err := readSplitRows(ctx, tx, `SELECT id::text, content_md, frontmatter::text FROM document_versions WHERE frontmatter <> '{}'::jsonb`)
	if err != nil {
		return err
	}
	for _, r := range verRows {
		whole, err := wholeMarkdown(r)
		if err != nil {
			return fmt.Errorf("document version %s: %w", r.id, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE document_versions SET content_md = $1 WHERE id = $2::uuid`, whole, r.id); err != nil {
			return fmt.Errorf("document version %s: %w", r.id, err)
		}
	}
	return nil
}

// downDocumentsWholeMarkdown splits every stored document back into
// body + JSONB frontmatter, which is what the previous code expects.
func downDocumentsWholeMarkdown(ctx context.Context, tx *sql.Tx) error {
	tables := []struct{ name, selectAll, update string }{
		{"documents",
			`SELECT id::text, content_md, frontmatter::text FROM documents`,
			`UPDATE documents SET content_md = $1 WHERE id = $2::uuid`},
		{"document_versions",
			`SELECT id::text, content_md, frontmatter::text FROM document_versions`,
			`UPDATE document_versions SET content_md = $1 WHERE id = $2::uuid`},
	}
	for _, table := range tables {
		rows, err := readSplitRows(ctx, tx, table.selectAll)
		if err != nil {
			return err
		}
		for _, r := range rows {
			fm, body, err := docs.SplitFrontmatter(r.content)
			if err != nil || fm == nil {
				continue
			}
			if _, err := tx.ExecContext(ctx, table.update, body, r.id); err != nil {
				return fmt.Errorf("%s %s: %w", table.name, r.id, err)
			}
		}
	}
	return nil
}

// readSplitRows loads every row first: database/sql cannot run the
// UPDATEs on the same transaction while a result set is still open.
func readSplitRows(ctx context.Context, tx *sql.Tx, query string) ([]splitRow, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []splitRow
	for rows.Next() {
		var r splitRow
		var fm string
		if err := rows.Scan(&r.id, &r.content, &fm); err != nil {
			return nil, err
		}
		r.frontmatter = []byte(fm)
		out = append(out, r)
	}
	return out, rows.Err()
}

// wholeMarkdown rebuilds one document and proves the result parses back
// to exactly what was stored.
func wholeMarkdown(r splitRow) (string, error) {
	// YAML is a superset of JSON; decoding through yaml keeps integers
	// as integers instead of turning them into float64.
	fm := map[string]any{}
	if err := yaml.Unmarshal(r.frontmatter, &fm); err != nil {
		return "", fmt.Errorf("decode frontmatter: %w", err)
	}
	encoded, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("encode frontmatter: %w", err)
	}
	whole := "---\n" + string(encoded) + "---\n\n" + r.content

	gotFM, gotBody, err := docs.SplitFrontmatter(whole)
	if err != nil {
		return "", fmt.Errorf("rebuilt document does not parse: %w", err)
	}
	if gotBody != r.content {
		return "", fmt.Errorf("rebuilt document does not round-trip its body")
	}
	again, err := yaml.Marshal(gotFM)
	if err != nil || string(again) != string(encoded) {
		return "", fmt.Errorf("rebuilt document does not round-trip its frontmatter")
	}
	return whole, nil
}
