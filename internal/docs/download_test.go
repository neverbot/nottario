package docs

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func testDownloadGrant(now time.Time) DownloadGrant {
	return DownloadGrant{
		ProjectID: uuid.MustParse("abcdef01-1111-1111-1111-111111111111"),
		Path:      "context/plan.md",
		Version:   3,
		UserID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		TokenID:   uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		ExpiresAt: now.Add(DownloadTTL).Unix(),
	}
}

func TestDownloadSignVerifyRoundTrip(t *testing.T) {
	now := time.Now()
	g := testDownloadGrant(now)
	q, err := SignDownload(testSecret, g)
	if err != nil {
		t.Fatal(err)
	}
	got, err := VerifyDownload(testSecret, q, now)
	if err != nil || got != g {
		t.Fatalf("VerifyDownload = %+v, %v; want %+v", got, err, g)
	}
}

func TestDownloadEveryBoundFieldIsProtected(t *testing.T) {
	now := time.Now()
	tamper := map[string]string{
		"p":    "44444444-4444-4444-4444-444444444444",
		"path": "context/other.md",
		"v":    "4",
		"u":    "55555555-5555-5555-5555-555555555555",
		"t":    "66666666-6666-6666-6666-666666666666",
	}
	for k, v := range tamper {
		q, _ := SignDownload(testSecret, testDownloadGrant(now))
		q.Set(k, v)
		if _, err := VerifyDownload(testSecret, q, now); !errors.Is(err, ErrDownloadSignature) {
			t.Errorf("tampered %s: %v, want signature error", k, err)
		}
	}
}

// An upload signature never verifies as a download, and the reverse:
// each purpose derives its own key.
func TestDownloadAndUploadKeysAreSeparate(t *testing.T) {
	if string(downloadKey(testSecret)) == string(uploadKey(testSecret)) {
		t.Fatal("download and upload share a key")
	}
}

func TestDownloadExpiryAndStrictness(t *testing.T) {
	now := time.Now()
	q, _ := SignDownload(testSecret, testDownloadGrant(now))
	if _, err := VerifyDownload(testSecret, q, now.Add(DownloadTTL+time.Minute)); !errors.Is(err, ErrDownloadExpired) {
		t.Errorf("late: %v, want expired", err)
	}
	if _, err := VerifyDownload([]byte("other"), q, now); !errors.Is(err, ErrDownloadSignature) {
		t.Errorf("wrong key: %v", err)
	}
	extra, _ := SignDownload(testSecret, testDownloadGrant(now))
	extra.Set("x", "1")
	if _, err := VerifyDownload(testSecret, extra, now); !errors.Is(err, ErrDownloadMalformed) {
		t.Errorf("unknown param: %v", err)
	}
	missing, _ := SignDownload(testSecret, testDownloadGrant(now))
	missing.Del("v")
	if _, err := VerifyDownload(testSecret, missing, now); !errors.Is(err, ErrDownloadMalformed) {
		t.Errorf("missing param: %v", err)
	}
	if _, err := SignDownload(nil, testDownloadGrant(now)); !errors.Is(err, ErrUploadNoKey) {
		t.Errorf("no key: %v", err)
	}
}
