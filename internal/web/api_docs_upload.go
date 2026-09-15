package web

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/identity"
)

// uploadClock is the time source for upload URL expiry. Tests move it.
var uploadClock = time.Now

// UploadDocHandler accepts a whole document at a URL signed by the
// nottario.docs.upload_url MCP tool (see internal/docs/upload.go).
//
// The signature is the only credential. Authorization headers and
// session cookies are ignored on purpose: this endpoint exists so that
// agents never have to handle a token, and accepting one here would
// quietly bring that back.
//
// Checks, in order, before a single body byte is read: signature,
// expiry, the requesting token still valid and still bound to the
// project and user in the grant, and the user still able to access the
// project. Then the body must hash to the signed file_sha256 and be
// valid UTF-8. The write itself runs the normal optimistic-concurrency
// check against the signed expected_version, which is what makes a
// replayed URL fail with version_conflict.
func UploadDocHandler(pool *pgxpool.Pool, sessionKey []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(sessionKey) == 0 {
			writeError(w, http.StatusServiceUnavailable, docs.ErrUploadNoKey.Error())
			return
		}
		g, err := docs.VerifyUpload(sessionKey, r.URL.Query(), uploadClock())
		switch {
		case errors.Is(err, docs.ErrUploadExpired):
			writeError(w, http.StatusForbidden, "upload URL expired; request a new one with nottario.docs.upload_url")
			return
		case errors.Is(err, docs.ErrUploadSignature):
			writeError(w, http.StatusForbidden, err.Error())
			return
		case err != nil:
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		ctx := r.Context()
		tok, err := identity.GetToken(ctx, pool, g.TokenID)
		if err != nil || tok.RevokedAt != nil || tok.ProjectID != g.ProjectID || tok.UserID != g.UserID {
			writeError(w, http.StatusForbidden, "the token that requested this upload URL is no longer valid for this project")
			return
		}
		user, err := identity.GetUser(ctx, pool, g.UserID)
		if err != nil {
			writeError(w, http.StatusForbidden, "the user that requested this upload URL no longer exists")
			return
		}
		if !user.IsAdmin {
			roles, err := identity.UserRoleIDs(ctx, pool, g.UserID, g.ProjectID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if len(roles) == 0 {
				writeError(w, http.StatusForbidden, "not a project member")
				return
			}
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
				return
			}
			writeError(w, http.StatusBadRequest, "could not read request body")
			return
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != g.FileSHA256 {
			writeError(w, http.StatusBadRequest, "body does not match the file_sha256 this URL was signed for; nothing was written")
			return
		}
		if !utf8.Valid(body) {
			writeError(w, http.StatusBadRequest, "document is not valid UTF-8; nothing was written")
			return
		}

		expected := g.ExpectedVersion
		doc, err := docs.Write(ctx, pool, docs.WriteParams{
			Scope:           docs.ScopeProject,
			ProjectID:       &g.ProjectID,
			Path:            g.Path,
			ContentMD:       string(body),
			Kind:            docs.Kind(g.Kind),
			Message:         g.Message,
			ExpectedVersion: &expected,
		}, docs.Authorship{UserID: &g.UserID, TokenID: &g.TokenID})
		var conflict *docs.VersionConflictError
		if errors.As(err, &conflict) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":           "version_conflict",
				"current_version": conflict.CurrentVersion,
				"message":         "the document changed since this URL was signed (or the URL was already used); run docs.stat and request a new URL",
			})
			return
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"path":            doc.Path,
			"current_version": doc.CurrentVersion,
			"updated_at":      doc.UpdatedAt,
		})
	})
}
