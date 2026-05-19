package web

import (
	"encoding/json"
	"net/http"

	"github.com/neverbot/nottario/internal/version"
)

// HealthzHandler returns 200 with a JSON body { "status": "ok" }.
// Reserved for liveness probes; readiness will be a separate
// endpoint that checks Postgres in a later milestone.
func HealthzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

// VersionHandler returns the build metadata.
func VersionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"version": version.Version,
			"commit":  version.Commit,
			"date":    version.Date,
		})
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
