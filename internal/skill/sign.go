package skill

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// SignZipURL returns an absolute URL with `exp` + `sig` query params
// that the /skill.zip handler accepts for the next ttl. The signature
// is HMAC-SHA256 over the decimal expiry timestamp keyed by the
// caller-supplied secret (the server reuses the session signing key
// so both rotate together). The signed-URL path lets agents fetch
// the bundle with any local HTTP tool without echoing their Bearer
// token through the context window.
func SignZipURL(baseURL string, key []byte, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	sig := signZipExpiry(key, exp)
	sep := "?"
	if strings.Contains(baseURL, "?") {
		sep = "&"
	}
	return baseURL + sep + "exp=" + strconv.FormatInt(exp, 10) + "&sig=" + sig
}

func signZipExpiry(key []byte, exp int64) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("skill.zip:" + strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyZipSig is the inverse of SignZipURL: returns true when the
// (exp, sig) pair is well-formed, the signature matches and the
// expiry is in the future. Constant-time compare on the signature.
func VerifyZipSig(key []byte, expStr, sig string) bool {
	if expStr == "" || sig == "" {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix() > exp {
		return false
	}
	want := signZipExpiry(key, exp)
	return hmac.Equal([]byte(want), []byte(sig))
}
