package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/neverbot/nottario/internal/skill"
)

// SkillHandler serves files from the bundled skill tree at
// /skill/<path>. The endpoint is unauthenticated so that agents can
// fetch the skill before they hold a token; nothing in the bundle is
// sensitive.
func SkillHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/skill" || r.URL.Path == "/skill/" {
			data, err := skill.Read("skill.md")
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/skill/")
		if path == "" {
			path = "skill.md"
		}
		data, err := skill.Read(path)
		if err != nil {
			status := http.StatusNotFound
			if !errors.Is(err, skill.ErrNotFound) {
				status = http.StatusBadRequest
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
}
