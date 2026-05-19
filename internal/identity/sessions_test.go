package identity

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCookieRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	id := uuid.New()
	encoded := EncodeCookie(id, key)
	got, err := DecodeCookie(encoded, key)
	if err != nil {
		t.Fatalf("DecodeCookie: %v", err)
	}
	if got != id {
		t.Errorf("decoded id = %v, want %v", got, id)
	}
}

func TestCookieTamperRejected(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	encoded := EncodeCookie(uuid.New(), key)
	// flip the last hex digit of the signature
	idx := strings.LastIndex(encoded, ".") + 1
	if idx == 0 || idx >= len(encoded) {
		t.Fatal("bad encoded shape")
	}
	tampered := encoded[:len(encoded)-1] + "0"
	if encoded == tampered {
		tampered = encoded[:len(encoded)-1] + "1"
	}
	if _, err := DecodeCookie(tampered, key); err == nil {
		t.Fatal("expected error for tampered cookie")
	}
}

func TestCookieWrongKeyRejected(t *testing.T) {
	keyA := []byte("0123456789abcdef0123456789abcdef")
	keyB := []byte("ffffffffffffffffffffffffffffffff")
	encoded := EncodeCookie(uuid.New(), keyA)
	if _, err := DecodeCookie(encoded, keyB); err == nil {
		t.Fatal("expected error when verifying with different key")
	}
}

func TestCookieMalformed(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	for _, bad := range []string{"", "nodot", "x.y", ".sig", "id."} {
		if _, err := DecodeCookie(bad, key); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
