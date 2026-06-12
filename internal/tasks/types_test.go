package tasks

import "testing"

func TestValidType(t *testing.T) {
	cases := map[Type]bool{
		TypeTask:     true,
		TypeBug:      true,
		TypeChore:    true,
		TypeSpike:    true,
		TypeFeature:  true,
		Type("nope"): false,
		Type(""):     false,
	}
	for in, want := range cases {
		if ValidType(in) != want {
			t.Errorf("ValidType(%q) = %v, want %v", in, !want, want)
		}
	}
}

func TestValidState(t *testing.T) {
	cases := map[State]bool{
		StateTodo:    true,
		StateDoing:   true,
		StateDone:    true,
		StateWontDo:  true,
		State("wip"): false,
		State(""):    false,
	}
	for in, want := range cases {
		if ValidState(in) != want {
			t.Errorf("ValidState(%q) = %v, want %v", in, !want, want)
		}
	}
}

func TestIsClosed(t *testing.T) {
	cases := map[State]bool{
		StateTodo:   false,
		StateDoing:  false,
		StateDone:   true,
		StateWontDo: true,
	}
	for in, want := range cases {
		if got := IsClosed(in); got != want {
			t.Errorf("IsClosed(%q) = %v, want %v", in, got, want)
		}
	}
}
