package web

import (
	"archive/zip"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neverbot/nottario/internal/skill"
)

// SkillHandler serves files from the skill tree at /skill/<path>.
// Override resolution: a document at global/skills/<path> takes
// precedence over the file embedded in the binary. The endpoint is
// unauthenticated; nothing in the bundle is sensitive.
func SkillHandler(pool *pgxpool.Pool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := "skill.md"
		if r.URL.Path != "/skill" && r.URL.Path != "/skill/" {
			path = strings.TrimPrefix(r.URL.Path, "/skill/")
			if path == "" {
				path = "skill.md"
			}
		}
		data, origin, err := skill.Read(r.Context(), pool, path)
		if err != nil {
			status := http.StatusNotFound
			if !errors.Is(err, skill.ErrNotFound) {
				status = http.StatusBadRequest
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("X-Nottario-Skill-Origin", string(origin))
		_, _ = w.Write(data)
	})
}

// SkillZipHandler streams the entire current skill tree (with
// overrides applied) as a single zip archive. Convenient for users
// who want to drop the skill into ~/.claude/skills/nottario/
// offline or as a backup.
func SkillZipHandler(pool *pgxpool.Pool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entries, err := skill.List(r.Context(), pool)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="nottario-skill.zip"`)

		zw := zip.NewWriter(w)
		defer zw.Close()
		now := time.Now()
		for _, e := range entries {
			data, _, err := skill.Read(r.Context(), pool, e.Path)
			if err != nil {
				continue
			}
			fw, err := zw.CreateHeader(&zip.FileHeader{
				Name:     e.Path,
				Method:   zip.Deflate,
				Modified: now,
			})
			if err != nil {
				continue
			}
			_, _ = fw.Write(data)
		}
	})
}
