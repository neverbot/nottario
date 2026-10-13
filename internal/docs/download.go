package docs

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Signed download URLs are the read side of upload URLs: an agent
// fetches one version of a document straight to a file on disk, so the
// bytes never cross its context unless it chooses to read them, and it
// never handles an API token.
//
// The MCP tool nottario.docs.download_url signs a DownloadGrant for the
// current version; the agent GETs the URL; the /api/docs/download
// handler verifies the grant and serves exactly that version. The grant
// binds project, path, version, the user and token that asked for it,
// and the expiry. Reading is idempotent, so the URL may be fetched more
// than once until it expires.
//
// Why it is safe to hand this URL to an agent: it expires after
// DownloadTTL, it names one version of one document, every field is
// inside the MAC (length-prefixed, as for uploads), the key is derived
// for downloads only so no other signature verifies as one, and the
// handler re-checks that the token is live and the user still a member
// before serving a byte.

// DownloadTTL is how long a signed download URL stays valid.
const DownloadTTL = 5 * time.Minute

var (
	// ErrDownloadMalformed covers missing, duplicated, unknown or
	// non-canonical parameters.
	ErrDownloadMalformed = errors.New("malformed download URL")
	// ErrDownloadSignature means the signature does not match.
	ErrDownloadSignature = errors.New("invalid download URL signature")
	// ErrDownloadExpired means a correctly signed URL is past its expiry.
	ErrDownloadExpired = errors.New("download URL expired")
)

// DownloadGrant is the signed permission to read one document version.
type DownloadGrant struct {
	ProjectID uuid.UUID
	Path      string
	Version   int
	UserID    uuid.UUID
	TokenID   uuid.UUID
	ExpiresAt int64 // unix seconds
}

func (g DownloadGrant) validate() error {
	if g.ProjectID == uuid.Nil || g.UserID == uuid.Nil || g.TokenID == uuid.Nil {
		return ErrDownloadMalformed
	}
	if g.Path == "" || g.Version < 1 || g.ExpiresAt <= 0 {
		return ErrDownloadMalformed
	}
	return nil
}

func downloadKey(secret []byte) []byte {
	m := hmac.New(sha256.New, secret)
	_, _ = m.Write([]byte("nottario/docs-download/v1"))
	return m.Sum(nil)
}

func (g DownloadGrant) mac(secret []byte) []byte {
	fields := []string{
		"nottario-docs-download-v1",
		g.ProjectID.String(),
		g.Path,
		strconv.Itoa(g.Version),
		g.UserID.String(),
		g.TokenID.String(),
		strconv.FormatInt(g.ExpiresAt, 10),
	}
	var b bytes.Buffer
	for _, f := range fields {
		b.WriteString(strconv.Itoa(len(f)))
		b.WriteByte(':')
		b.WriteString(f)
	}
	m := hmac.New(sha256.New, downloadKey(secret))
	_, _ = m.Write(b.Bytes())
	return m.Sum(nil)
}

var downloadParams = []string{"p", "path", "v", "u", "t", "exp", "sig"}

// SignDownload returns the query parameters of a signed download URL.
func SignDownload(secret []byte, g DownloadGrant) (url.Values, error) {
	if len(secret) == 0 {
		return nil, ErrUploadNoKey
	}
	if err := g.validate(); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("p", g.ProjectID.String())
	q.Set("path", g.Path)
	q.Set("v", strconv.Itoa(g.Version))
	q.Set("u", g.UserID.String())
	q.Set("t", g.TokenID.String())
	q.Set("exp", strconv.FormatInt(g.ExpiresAt, 10))
	q.Set("sig", hex.EncodeToString(g.mac(secret)))
	return q, nil
}

// VerifyDownload checks a signed download URL and returns its grant,
// with the same strictness as VerifyUpload: signature before expiry,
// every parameter exactly once, nothing unknown, canonical values.
func VerifyDownload(secret []byte, q url.Values, now time.Time) (DownloadGrant, error) {
	if len(secret) == 0 {
		return DownloadGrant{}, ErrUploadNoKey
	}
	known := map[string]bool{}
	for _, k := range downloadParams {
		known[k] = true
		if len(q[k]) != 1 {
			return DownloadGrant{}, ErrDownloadMalformed
		}
	}
	for k := range q {
		if !known[k] {
			return DownloadGrant{}, ErrDownloadMalformed
		}
	}
	parseUUID := func(s string) (uuid.UUID, bool) {
		id, err := uuid.Parse(s)
		return id, err == nil && id.String() == s
	}
	pid, ok1 := parseUUID(q.Get("p"))
	uid, ok2 := parseUUID(q.Get("u"))
	tid, ok3 := parseUUID(q.Get("t"))
	v, errV := strconv.Atoi(q.Get("v"))
	exp, errE := strconv.ParseInt(q.Get("exp"), 10, 64)
	sig, errS := hex.DecodeString(q.Get("sig"))
	if !ok1 || !ok2 || !ok3 || errV != nil || errE != nil || errS != nil ||
		strconv.Itoa(v) != q.Get("v") || strconv.FormatInt(exp, 10) != q.Get("exp") {
		return DownloadGrant{}, ErrDownloadMalformed
	}
	g := DownloadGrant{ProjectID: pid, Path: q.Get("path"), Version: v, UserID: uid, TokenID: tid, ExpiresAt: exp}
	if err := g.validate(); err != nil {
		return DownloadGrant{}, err
	}
	if !hmac.Equal(g.mac(secret), sig) {
		return DownloadGrant{}, ErrDownloadSignature
	}
	if now.Unix() > g.ExpiresAt {
		return DownloadGrant{}, ErrDownloadExpired
	}
	if time.Unix(g.ExpiresAt, 0).After(now.Add(DownloadTTL + uploadClockSkew)) {
		return DownloadGrant{}, ErrDownloadMalformed
	}
	return g, nil
}
