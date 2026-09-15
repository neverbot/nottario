package docs

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Signed upload URLs let an agent move a whole document from disk to
// the server without the bytes crossing its context and without ever
// handling an API token.
//
// The MCP tool nottario.docs.upload_url signs an UploadGrant; the agent
// PUTs the exact file bytes to the URL; the /api/docs/upload handler
// verifies the grant and writes. The signature IS the credential for
// that one write, so the grant binds everything the write depends on:
// project, path, the version the caller expects to replace, the SHA-256
// of the bytes it will send, kind and message, the user and token that
// asked for it, and the expiry.
//
// Why it is safe to hand this URL to an agent:
//
//   - It expires after UploadTTL (5 minutes).
//   - It authorises exactly one outcome: those bytes, at that path, on
//     top of that version. A different body fails the hash; a replay
//     fails optimistic concurrency, because the first successful write
//     already moved current_version past expected_version. That makes
//     each URL effectively single-use without a nonce table.
//   - It cannot be widened: every field is inside the MAC, encoded with
//     length prefixes so no value can be shifted into its neighbour.
//   - It carries no reusable secret. Leaking it in a transcript or a
//     proxy log exposes, at most, a few minutes of permission to write
//     content the leaker would also need to possess.

// UploadTTL is how long a signed upload URL stays valid.
const UploadTTL = 5 * time.Minute

// uploadClockSkew is the tolerance applied when rejecting grants whose
// expiry lies further in the future than this server would ever sign.
const uploadClockSkew = 30 * time.Second

var (
	// ErrUploadNoKey means the server has no signing secret; upload
	// URLs are never issued or accepted unsigned.
	ErrUploadNoKey = errors.New("document uploads are disabled: the server has no signing key")
	// ErrUploadMalformed covers missing, duplicated, unknown or
	// non-canonical parameters.
	ErrUploadMalformed = errors.New("malformed upload URL")
	// ErrUploadSignature means the signature does not match the
	// parameters.
	ErrUploadSignature = errors.New("invalid upload URL signature")
	// ErrUploadExpired means a correctly signed URL is past its expiry.
	ErrUploadExpired = errors.New("upload URL expired")
)

var sha256HexRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidSHA256Hex reports whether s is a lowercase 64-character hex
// SHA-256 digest.
func ValidSHA256Hex(s string) bool { return sha256HexRE.MatchString(s) }

// UploadGrant is the signed permission to write one document once.
type UploadGrant struct {
	ProjectID       uuid.UUID
	Path            string
	ExpectedVersion int
	FileSHA256      string
	Kind            string
	Message         string
	UserID          uuid.UUID
	TokenID         uuid.UUID
	ExpiresAt       int64 // unix seconds
}

func (g UploadGrant) validate() error {
	if g.ProjectID == uuid.Nil || g.UserID == uuid.Nil || g.TokenID == uuid.Nil {
		return ErrUploadMalformed
	}
	if g.Path == "" || g.ExpectedVersion < 0 || !ValidSHA256Hex(g.FileSHA256) || g.ExpiresAt <= 0 {
		return ErrUploadMalformed
	}
	return nil
}

// uploadKey derives a key used only for upload grants, so a signature
// made for any other purpose with the same server secret (session
// cookies, skill.zip URLs) can never verify as an upload grant.
func uploadKey(secret []byte) []byte {
	m := hmac.New(sha256.New, secret)
	_, _ = m.Write([]byte("nottario/docs-upload/v1"))
	return m.Sum(nil)
}

// mac computes the grant's signature over a canonical, length-prefixed
// encoding of every field.
func (g UploadGrant) mac(secret []byte) []byte {
	fields := []string{
		"nottario-docs-upload-v1",
		g.ProjectID.String(),
		g.Path,
		strconv.Itoa(g.ExpectedVersion),
		g.FileSHA256,
		g.Kind,
		g.Message,
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
	m := hmac.New(sha256.New, uploadKey(secret))
	_, _ = m.Write(b.Bytes())
	return m.Sum(nil)
}

// SignUpload returns the query parameters of a signed upload URL.
func SignUpload(secret []byte, g UploadGrant) (url.Values, error) {
	if len(secret) == 0 {
		return nil, ErrUploadNoKey
	}
	if err := g.validate(); err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("p", g.ProjectID.String())
	q.Set("path", g.Path)
	q.Set("v", strconv.Itoa(g.ExpectedVersion))
	q.Set("h", g.FileSHA256)
	q.Set("k", g.Kind)
	q.Set("m", g.Message)
	q.Set("u", g.UserID.String())
	q.Set("t", g.TokenID.String())
	q.Set("exp", strconv.FormatInt(g.ExpiresAt, 10))
	q.Set("sig", hex.EncodeToString(g.mac(secret)))
	return q, nil
}

func isUploadParam(k string) bool {
	switch k {
	case "p", "path", "v", "h", "k", "m", "u", "t", "exp", "sig":
		return true
	}
	return false
}

// VerifyUpload checks a signed upload URL and returns its grant.
//
// The signature is checked before the expiry, so a caller without a
// valid signature learns nothing about whether the URL would otherwise
// have been live. Parameters must appear exactly once, no unknown
// parameter is accepted, and numeric and UUID values must be in the
// exact canonical form the signer produced.
func VerifyUpload(secret []byte, q url.Values, now time.Time) (UploadGrant, error) {
	if len(secret) == 0 {
		return UploadGrant{}, ErrUploadNoKey
	}
	for k, vals := range q {
		if !isUploadParam(k) || len(vals) != 1 {
			return UploadGrant{}, ErrUploadMalformed
		}
	}
	for _, k := range []string{"p", "path", "v", "h", "k", "m", "u", "t", "exp", "sig"} {
		if _, ok := q[k]; !ok {
			return UploadGrant{}, ErrUploadMalformed
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
		return UploadGrant{}, ErrUploadMalformed
	}

	g := UploadGrant{
		ProjectID:       pid,
		Path:            q.Get("path"),
		ExpectedVersion: v,
		FileSHA256:      q.Get("h"),
		Kind:            q.Get("k"),
		Message:         q.Get("m"),
		UserID:          uid,
		TokenID:         tid,
		ExpiresAt:       exp,
	}
	if err := g.validate(); err != nil {
		return UploadGrant{}, err
	}
	if !hmac.Equal(g.mac(secret), sig) {
		return UploadGrant{}, ErrUploadSignature
	}
	if now.Unix() > g.ExpiresAt {
		return UploadGrant{}, ErrUploadExpired
	}
	// Only this server signs grants, and it never signs further out
	// than UploadTTL. A later expiry means a key was misused.
	if time.Unix(g.ExpiresAt, 0).After(now.Add(UploadTTL + uploadClockSkew)) {
		return UploadGrant{}, ErrUploadMalformed
	}
	return g, nil
}
