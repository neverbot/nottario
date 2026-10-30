package mcp

import (
	"reflect"
	"testing"
)

func TestWithSessionError(t *testing.T) {
	type payload struct {
		A string `json:"a"`
		B int    `json:"b,omitempty"`
	}
	in := payload{A: "x"}
	if got := withSessionError(in, ""); !reflect.DeepEqual(got, in) {
		t.Errorf("empty message changed the payload: %#v", got)
	}
	got, ok := withSessionError(payload{A: "x", B: 2}, "boom").(map[string]any)
	if !ok {
		t.Fatal("payload is not a map")
	}
	want := map[string]any{"a": "x", "b": float64(2), "session_error": "boom"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestSafeToken(t *testing.T) {
	if got := safeToken("claude code/1<b>"); got != "claudecode1b" {
		t.Errorf("safeToken = %q", got)
	}
}
