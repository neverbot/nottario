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
// overrides applied) as a single zip archive. Two auth modes:
//
//   - Bearer token (existing): for the web UI and arbitrary scripts
//     that already hold credentials.
//   - Signed query params (?exp=&sig=): for agents that fetched a
//     time-bound URL from `nottario.skill.install`. The signature is
//     HMAC-SHA256 keyed by the server's session signing key, with a
//     5-minute TTL embedded in `exp`.
//
// When neither is supplied the handler stays unauthenticated, matching
// historical behaviour. Tighten in a follow-up if we ever decide the
// bundle is sensitive — for now it isn't.
func SkillZipHandler(pool *pgxpool.Pool, sessionKey []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exp := r.URL.Query().Get("exp")
		sig := r.URL.Query().Get("sig")
		if exp != "" || sig != "" {
			// Caller opted into the signed-URL path. Validate strictly:
			// a malformed or expired URL must NOT silently fall back to
			// "no auth required".
			if !skill.VerifyZipSig(sessionKey, exp, sig) {
				http.Error(w, "invalid or expired signature", http.StatusUnauthorized)
				return
			}
		}

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
