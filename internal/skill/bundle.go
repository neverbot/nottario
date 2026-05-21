// Package skill packages the bundled skill files into the binary and
// resolves them with per-organisation overrides.
//
// Resolution order for any requested path (e.g. "domains/tasks.md"):
//
//  1. A `documents` row with scope='global', kind='skill' and
//     path='global/skills/<requested>'. When present, its content
//     (frontmatter reconstructed from JSONB + body) is returned and
//     marked Origin="global".
//  2. Otherwise the file embedded in the binary, marked
//     Origin="embedded".
//
// Listings union the two sources, preferring `global` when both
// exist and surfacing global-only files (organisation-added skills).
package skill

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/db/dbq"
	"gopkg.in/yaml.v3"
)

//go:embed all:files
var bundled embed.FS

// ErrNotFound is returned when no override and no embedded file matches.
var ErrNotFound = errors.New("skill file not found")

// Origin tells the caller where the served content came from.
type Origin string

const (
	OriginEmbedded Origin = "embedded"
	OriginGlobal   Origin = "global"
)

// Entry is one file in the catalogue: its logical path and where it
// is served from for that path right now.
type Entry struct {
	Path   string `json:"path"`
	Origin Origin `json:"origin"`
}

// globalSkillPrefix is the prefix used by override documents in the
// documents table. The user-facing skill path "domains/tasks.md"
// becomes "global/skills/domains/tasks.md" as a document path.
const globalSkillPrefix = "global/skills/"

// Embedded reads a file from the bundled skill tree only.
func Embedded(path string) ([]byte, error) {
	clean, err := safePath(path)
	if err != nil {
		return nil, err
	}
	data, err := bundled.ReadFile("files/" + clean)
	if err != nil {
		return nil, ErrNotFound
	}
	return data, nil
}

// Read resolves a skill file with overrides. When the user has
// written a `kind=skill` document at `global/skills/<path>`, that
// content is returned with frontmatter reconstructed; otherwise the
// embedded copy.
func Read(ctx context.Context, pool *pgxpool.Pool, path string) ([]byte, Origin, error) {
	clean, err := safePath(path)
	if err != nil {
		return nil, "", err
	}
	if pool != nil {
		if body, ok := readOverride(ctx, pool, clean); ok {
			return body, OriginGlobal, nil
		}
	}
	body, err := bundled.ReadFile("files/" + clean)
	if err != nil {
		return nil, "", ErrNotFound
	}
	return body, OriginEmbedded, nil
}

// List returns every file currently available (embedded + global
// overrides + global-only additions), sorted by path.
func List(ctx context.Context, pool *pgxpool.Pool) ([]Entry, error) {
	out := map[string]Origin{}

	// Embedded.
	err := fs.WalkDir(bundled, "files", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, "files/")
		out[rel] = OriginEmbedded
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Global overrides (and global-only additions).
	if pool != nil {
		paths, qerr := dbq.New(pool).ListSkillOverridePaths(ctx, globalSkillPrefix+"%")
		if qerr == nil {
			for _, p := range paths {
				rel := strings.TrimPrefix(p, globalSkillPrefix)
				out[rel] = OriginGlobal
			}
		}
	}

	entries := make([]Entry, 0, len(out))
	for p, o := range out {
		entries = append(entries, Entry{Path: p, Origin: o})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// readOverride looks up the document at global/skills/<path>. When
// found, it reconstructs the markdown (frontmatter YAML + body) and
// returns it.
func readOverride(ctx context.Context, pool *pgxpool.Pool, path string) ([]byte, bool) {
	row, err := dbq.New(pool).GetSkillOverride(ctx, globalSkillPrefix+path)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false
	}
	if err != nil {
		return nil, false
	}
	body := row.ContentMd
	rawFM := row.Frontmatter

	// Reconstruct: if the document has a non-empty frontmatter object,
	// emit it as YAML at the top followed by the body.
	if len(rawFM) > 0 && string(rawFM) != "{}" {
		var fm map[string]any
		if err := yamlUnmarshalJSON(rawFM, &fm); err == nil && len(fm) > 0 {
			fmYaml, err := yaml.Marshal(fm)
			if err == nil {
				combined := fmt.Sprintf("---\n%s---\n\n%s", string(fmYaml), body)
				return []byte(combined), true
			}
		}
	}
	return []byte(body), true
}

// yamlUnmarshalJSON parses JSON-encoded bytes (what jsonb returns)
// into a map[string]any. We use yaml.Unmarshal because YAML is a
// JSON superset and the yaml.v3 decoder happens to handle JSON
// fine; this avoids pulling in encoding/json just for this helper.
func yamlUnmarshalJSON(raw []byte, dst *map[string]any) error {
	return yaml.Unmarshal(raw, dst)
}

// safePath rejects path traversal and leading slashes.
func safePath(p string) (string, error) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", errors.New("path is empty")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", errors.New("invalid path segment")
		}
	}
	return p, nil
}
