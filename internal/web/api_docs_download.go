package web

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/docs"
)

// downloadClock is the time source for download URL expiry. Tests move it.
var downloadClock = time.Now

// DownloadDocHandler serves one document version at a URL signed by the
// nottario.docs.download_url MCP tool (see internal/docs/download.go).
// As with uploads, the signature is the only credential: Authorization
// headers and session cookies are ignored. The body is the document
// exactly as stored, frontmatter included, so its SHA-256 matches
// docs.stat's content_sha256 for that version.
func DownloadDocHandler(pool *pgxpool.Pool, sessionKey []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(sessionKey) == 0 {
			writeError(w, http.StatusServiceUnavailable, docs.ErrUploadNoKey.Error())
			return
		}
		g, err := docs.VerifyDownload(sessionKey, r.URL.Query(), downloadClock())
		switch {
		case errors.Is(err, docs.ErrDownloadExpired):
			writeError(w, http.StatusForbidden, "download URL expired; request a new one with nottario.docs.download_url")
			return
		case errors.Is(err, docs.ErrDownloadSignature):
			writeError(w, http.StatusForbidden, err.Error())
			return
		case err != nil:
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		ctx := r.Context()
		if status, msg := checkSignedCaller(ctx, pool, g.TokenID, g.ProjectID, g.UserID, "download"); status != 0 {
			writeError(w, status, msg)
			return
		}
		doc, err := docs.Read(ctx, pool, docs.ScopeProject, &g.ProjectID, g.Path)
		if errors.Is(err, docs.ErrNotFound) {
			writeError(w, http.StatusNotFound, "document not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		content := doc.ContentMD
		if doc.CurrentVersion != g.Version {
			v, err := docs.ReadVersion(ctx, pool, doc.ID, g.Version)
			if err != nil {
				writeError(w, http.StatusNotFound, "document version not found")
				return
			}
			content = v.ContentMD
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("X-Nottario-Version", strconv.Itoa(g.Version))
		w.Header().Set("Cache-Control", "private, no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	})
}
