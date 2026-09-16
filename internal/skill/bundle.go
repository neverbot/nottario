// Package skill packages the bundled skill files into the binary and
// resolves them with per-organisation overrides.
//
// Resolution order for any requested path (e.g. "domains/tasks.md"):
//
//  1. A `documents` row with scope='global', kind='skill' and
//     path='global/skills/<requested>'. When present, its content is
//     returned exactly as stored and marked Origin="global".
//  2. Otherwise the file embedded in the binary, marked
//     Origin="embedded".
//
// Listings union the two sources, preferring `global` when both
// exist and surfacing global-only files (organisation-added skills).
package skill

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/db/dbq"
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
// content is returned as stored; otherwise the embedded copy.
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

// ManifestName is the checksum file shipped at the root of the bundle
// zip. It lists every other file as `<sha256 hex>  <path>`, sorted by
// path: the format `sha256sum` / `shasum -a 256` print and accept with
// `-c`.
const ManifestName = "SHA256SUMS"

// File is one resolved bundle file.
type File struct {
	Path string
	Data []byte
}

// Snapshot resolves every bundle file once (overrides applied) and
// builds the manifest from exactly those bytes, so the zip and
// bundle_version always describe the same content.
func Snapshot(ctx context.Context, pool *pgxpool.Pool) ([]File, []byte, error) {
	entries, err := List(ctx, pool)
	if err != nil {
		return nil, nil, err
	}
	files := make([]File, 0, len(entries))
	var manifest strings.Builder
	for _, e := range entries {
		if e.Path == ManifestName {
			// Reserved for the manifest itself; an override with this
			// name would make the bundle describe itself.
			continue
		}
		data, _, err := Read(ctx, pool, e.Path)
		if err != nil {
			return nil, nil, err
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&manifest, "%s  %s\n", hex.EncodeToString(sum[:]), e.Path)
		files = append(files, File{Path: e.Path, Data: data})
	}
	return files, []byte(manifest.String()), nil
}

// VersionOf returns the bundle_version for a manifest: "sha256:" plus
// the SHA-256 of the SHA256SUMS file. An installed bundle checks itself
// with `shasum -a 256 <dir>/SHA256SUMS`; it is not a hash of the zip.
func VersionOf(manifest []byte) string {
	sum := sha256.Sum256(manifest)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// BundleVersion returns the version of the bundle currently served.
func BundleVersion(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	_, manifest, err := Snapshot(ctx, pool)
	if err != nil {
		return "", err
	}
	return VersionOf(manifest), nil
}

// readOverride looks up the document at global/skills/<path> and
// returns it exactly as it was written.
func readOverride(ctx context.Context, pool *pgxpool.Pool, path string) ([]byte, bool) {
	content, err := dbq.New(pool).GetSkillOverride(ctx, globalSkillPrefix+path)
	if err != nil {
		return nil, false
	}
	return []byte(content), true
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
