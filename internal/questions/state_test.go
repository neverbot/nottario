package questions_test

import (
	"testing"

	"github.com/neverbot/nottario/internal/questions"
)

func TestParseState(t *testing.T) {
	for in, want := range map[string]string{"open": "open", " Answered ": "answered", "withdrawn": "withdrawn", "all": ""} {
		got, err := questions.ParseState(in)
		if err != nil {
			t.Fatalf("ParseState(%q): %v", in, err)
		}
		if (got == nil) != (want == "") || (got != nil && string(*got) != want) {
			t.Fatalf("ParseState(%q) = %v, want %q", in, got, want)
		}
	}
	if _, err := questions.ParseState("pending"); !questions.IsInvalid(err) {
		t.Fatalf("ParseState(pending) err = %v, want invalid", err)
	}
}
