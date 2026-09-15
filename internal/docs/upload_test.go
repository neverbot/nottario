package docs

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var testSecret = []byte("unit-test-secret")

func testGrant(now time.Time) UploadGrant {
	return UploadGrant{
		ProjectID:       uuid.MustParse("abcdef01-1111-1111-1111-111111111111"),
		Path:            "context/plan.md",
		ExpectedVersion: 3,
		FileSHA256:      strings.Repeat("ab", 32),
		Kind:            "context",
		Message:         "sync",
		UserID:          uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		TokenID:         uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		ExpiresAt:       now.Add(UploadTTL).Unix(),
	}
}

func TestUploadTTLIsFiveMinutes(t *testing.T) {
	if UploadTTL != 5*time.Minute {
		t.Fatalf("UploadTTL = %v, want 5m", UploadTTL)
	}
}

func TestUploadSignVerifyRoundTrip(t *testing.T) {
	now := time.Now()
	g := testGrant(now)
	q, err := SignUpload(testSecret, g)
	if err != nil {
		t.Fatalf("SignUpload: %v", err)
	}
	got, err := VerifyUpload(testSecret, q, now)
	if err != nil {
		t.Fatalf("VerifyUpload: %v", err)
	}
	if got != g {
		t.Errorf("grant changed in transit: %+v vs %+v", got, g)
	}
}

func TestUploadEveryBoundFieldIsProtected(t *testing.T) {
	now := time.Now()
	q, _ := SignUpload(testSecret, testGrant(now))
	tamper := map[string]string{
		"p":    "44444444-4444-4444-4444-444444444444",
		"path": "context/other.md",
		"v":    "4",
		"h":    strings.Repeat("cd", 32),
		"k":    "skill",
		"m":    "different",
		"u":    "55555555-5555-5555-5555-555555555555",
		"t":    "66666666-6666-6666-6666-666666666666",
		"exp":  "9999999999",
	}
	for k, v := range tamper {
		t.Run(k, func(t *testing.T) {
			mod := url.Values{}
			for kk, vv := range q {
				mod[kk] = append([]string(nil), vv...)
			}
			mod.Set(k, v)
			_, err := VerifyUpload(testSecret, mod, now)
			if !errors.Is(err, ErrUploadSignature) {
				t.Errorf("tampered %q: err = %v, want ErrUploadSignature", k, err)
			}
		})
	}
}

func TestUploadWrongKeyRejected(t *testing.T) {
	now := time.Now()
	q, _ := SignUpload(testSecret, testGrant(now))
	if _, err := VerifyUpload([]byte("another-secret"), q, now); !errors.Is(err, ErrUploadSignature) {
		t.Errorf("err = %v, want ErrUploadSignature", err)
	}
}

func TestUploadExpiry(t *testing.T) {
	now := time.Now()
	q, _ := SignUpload(testSecret, testGrant(now))
	if _, err := VerifyUpload(testSecret, q, now.Add(UploadTTL-time.Second)); err != nil {
		t.Errorf("still valid just before expiry: %v", err)
	}
	if _, err := VerifyUpload(testSecret, q, now.Add(UploadTTL+2*time.Second)); !errors.Is(err, ErrUploadExpired) {
		t.Errorf("after expiry: err = %v, want ErrUploadExpired", err)
	}
}

// A bad signature must be reported as such even when the URL is also
// expired: expiry is only revealed to holders of a valid signature.
func TestUploadSignatureCheckedBeforeExpiry(t *testing.T) {
	now := time.Now()
	q, _ := SignUpload(testSecret, testGrant(now))
	q.Set("path", "context/elsewhere.md")
	if _, err := VerifyUpload(testSecret, q, now.Add(time.Hour)); !errors.Is(err, ErrUploadSignature) {
		t.Errorf("err = %v, want ErrUploadSignature", err)
	}
}

func TestUploadExpiryBeyondTTLRejectedEvenIfSigned(t *testing.T) {
	now := time.Now()
	g := testGrant(now)
	g.ExpiresAt = now.Add(time.Hour).Unix()
	q, _ := SignUpload(testSecret, g)
	if _, err := VerifyUpload(testSecret, q, now); !errors.Is(err, ErrUploadMalformed) {
		t.Errorf("err = %v, want ErrUploadMalformed", err)
	}
}

func TestUploadStrictParameters(t *testing.T) {
	now := time.Now()
	base, _ := SignUpload(testSecret, testGrant(now))
	clone := func() url.Values {
		c := url.Values{}
		for k, v := range base {
			c[k] = append([]string(nil), v...)
		}
		return c
	}
	cases := map[string]func(url.Values){
		"duplicate path":    func(q url.Values) { q.Add("path", "context/plan.md") },
		"unknown param":     func(q url.Values) { q.Set("scope", "global") },
		"missing hash":      func(q url.Values) { q.Del("h") },
		"leading zero v":    func(q url.Values) { q.Set("v", "03") },
		"plus sign v":       func(q url.Values) { q.Set("v", "+3") },
		"uppercase uuid":    func(q url.Values) { q.Set("p", strings.ToUpper(base.Get("p"))) },
		"non-hex signature": func(q url.Values) { q.Set("sig", "zz") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			q := clone()
			mutate(q)
			if _, err := VerifyUpload(testSecret, q, now); err == nil {
				t.Errorf("accepted a URL with %s", name)
			}
		})
	}
}

// Length prefixes stop a value from being shifted into its neighbour:
// ("ab", "c") and ("a", "bc") must not collide.
func TestUploadCanonicalEncodingIsUnambiguous(t *testing.T) {
	now := time.Now()
	a := testGrant(now)
	a.Kind, a.Message = "ab", "c"
	b := testGrant(now)
	b.Kind, b.Message = "a", "bc"
	if string(a.mac(testSecret)) == string(b.mac(testSecret)) {
		t.Fatal("different field splits produced the same MAC")
	}
}

func TestUploadRefusesWithoutKey(t *testing.T) {
	now := time.Now()
	if _, err := SignUpload(nil, testGrant(now)); !errors.Is(err, ErrUploadNoKey) {
		t.Errorf("sign without key: err = %v", err)
	}
	q, _ := SignUpload(testSecret, testGrant(now))
	if _, err := VerifyUpload(nil, q, now); !errors.Is(err, ErrUploadNoKey) {
		t.Errorf("verify without key: err = %v", err)
	}
}

func TestUploadSignRejectsInvalidGrants(t *testing.T) {
	now := time.Now()
	bad := []func(*UploadGrant){
		func(g *UploadGrant) { g.Path = "" },
		func(g *UploadGrant) { g.ExpectedVersion = -1 },
		func(g *UploadGrant) { g.FileSHA256 = "not-a-hash" },
		func(g *UploadGrant) { g.FileSHA256 = strings.Repeat("AB", 32) },
		func(g *UploadGrant) { g.TokenID = uuid.Nil },
	}
	for i, mutate := range bad {
		g := testGrant(now)
		mutate(&g)
		if _, err := SignUpload(testSecret, g); err == nil {
			t.Errorf("case %d: signed an invalid grant", i)
		}
	}
}
