package web

import (
	"net/http"
	"strings"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/selfupdate"
	"github.com/neverbot/nottario/internal/version"
)

// VersionStatusDeps wires the version-status handler. State is nil
// when SELF_UPDATE_CHECK_ENABLED=false; Upstream still carries the
// configured value for debug echo.
type VersionStatusDeps struct {
	Resolver *identity.Resolver
	State    *selfupdate.State
	Upstream string
}

// VersionStatusHandler returns the running build metadata and, for
// admins only, the latest upstream sha reported by the self-update
// poller. Non-admin members receive `update_available: false` and no
// `latest` block — the running commit sha is a mild info-leak we
// don't need to expose publicly, and non-admins can't `docker compose
// pull` anyway.
func VersionStatusHandler(d VersionStatusDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := d.Resolver.ResolveSession(r)
		if !ok {
			if c2, ok2 := d.Resolver.ResolveToken(r); ok2 {
				c = c2
				ok = true
			}
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		body := map[string]any{
			"running": map[string]string{
				"sha":      version.Commit,
				"version":  version.Version,
				"built_at": version.Date,
			},
			"latest":           nil,
			"update_available": false,
			"check_enabled":    d.State != nil,
		}

		if !c.IsAdmin {
			writeJSON(w, http.StatusOK, body)
			return
		}

		if d.Upstream != "" {
			body["upstream"] = d.Upstream
		}

		if d.State != nil {
			latestSHA, checkedAt, lastErr := d.State.Snapshot()
			// Only surface the `latest` block once the poller has
			// actually reported something — empty struct + zero time
			// would look like a broken response to the frontend.
			if latestSHA != "" || !checkedAt.IsZero() || lastErr != "" {
				latest := map[string]any{}
				if latestSHA != "" {
					latest["sha"] = latestSHA
				}
				if !checkedAt.IsZero() {
					latest["checked_at"] = checkedAt.UTC()
				}
				if lastErr != "" {
					latest["last_error"] = lastErr
				}
				body["latest"] = latest
			}
			body["update_available"] = updateAvailable(version.Commit, latestSHA)
		}

		writeJSON(w, http.StatusOK, body)
	})
}

// updateAvailable decides whether the two SHAs represent different
// commits. Blank on either side means "unknown", never "available"
// — we never scare the operator with a spurious banner during a
// startup window before the first successful check.
//
// The two inputs routinely differ in length: CI stamps the running
// sha via `git rev-parse --short HEAD` (7 chars) while the GitHub
// commits API returns the full 40-char sha. A naive full-string
// EqualFold would always report "different", pinning the banner ON
// for every self-hoster. Compare by the shorter of the two lengths
// so any short/full combo (or two shorts, or two fulls) works, as
// long as one is a prefix of the other.
func updateAvailable(runningSHA, latestSHA string) bool {
	if runningSHA == "" || latestSHA == "" || runningSHA == "none" {
		return false
	}
	n := len(runningSHA)
	if len(latestSHA) < n {
		n = len(latestSHA)
	}
	return !strings.EqualFold(runningSHA[:n], latestSHA[:n])
}
