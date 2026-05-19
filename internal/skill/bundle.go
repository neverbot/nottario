// Package skill packages the bundled skill files into the binary and
// exposes them through an http.FileSystem-style API used by both the
// MCP `nottario.skill.read` tool and the `/skill/...` HTTP endpoint.
package skill

import (
	"embed"
	"errors"
	"io/fs"
	"strings"
)

//go:embed all:files
var bundled embed.FS

// ErrNotFound is returned by Read when no skill file matches the path.
var ErrNotFound = errors.New("skill file not found")

// Read returns the contents of a bundled skill file. The path is
// relative to the bundle root (e.g. "skill.md", "references/identity.md").
// Path traversal is rejected.
func Read(path string) ([]byte, error) {
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

// List returns every bundled file path, suitable for advertising the
// skill catalogue. Paths are slash-separated and relative.
func List() ([]string, error) {
	var out []string
	err := fs.WalkDir(bundled, "files", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		out = append(out, strings.TrimPrefix(p, "files/"))
		return nil
	})
	return out, err
}

// safePath rejects path traversal attempts and leading slashes.
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
