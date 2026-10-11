package tasks_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

func stepTexts(steps []tasks.Step) []string {
	out := make([]string, 0, len(steps))
	for i, s := range steps {
		if s.Position != i {
			return append(out, fmt.Sprintf("<position %d at index %d>", s.Position, i))
		}
		out = append(out, s.Text)
	}
	return out
}

func TestSteps(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 9301, "steps", "Steps", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "StepsProj", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	by := tasks.Authorship{UserID: &u.ID}
	mk := func(title string) *tasks.Task {
		t.Helper()
		tk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: title}, by)
		if err != nil {
			t.Fatal(err)
		}
		return tk
	}
	add := func(tk *tasks.Task, texts ...string) []tasks.Step {
		t.Helper()
		got, err := tasks.AddSteps(ctx, pool, tk.ID, texts, by)
		if err != nil {
			t.Fatalf("AddSteps: %v", err)
		}
		return got
	}

	t.Run("add keeps order and trims", func(t *testing.T) {
		tk := mk("order")
		add(tk, " one ", "two")
		got := add(tk, "three")
		if want := []string{"one", "two", "three"}; !reflect.DeepEqual(stepTexts(got), want) {
			t.Errorf("steps = %v, want %v", stepTexts(got), want)
		}
	})

	t.Run("text is validated before anything is written", func(t *testing.T) {
		tk := mk("validation")
		for _, bad := range [][]string{
			{""},
			{"   "},
			{"two\nlines"},
			{strings.Repeat("x", tasks.MaxStepLen+1)},
			{"fine", ""},
			{},
		} {
			if _, err := tasks.AddSteps(ctx, pool, tk.ID, bad, by); err == nil {
				t.Errorf("AddSteps(%q) succeeded, want an error", bad)
			}
		}
		if got, _ := tasks.ListSteps(ctx, pool, tk.ID); len(got) != 0 {
			t.Errorf("rejected batches left %d steps", len(got))
		}
		if _, err := tasks.AddSteps(ctx, pool, tk.ID, []string{strings.Repeat("é", tasks.MaxStepLen)}, by); err != nil {
			t.Errorf("200 multibyte characters rejected: %v", err)
		}
	})

	t.Run("cap says how many fit", func(t *testing.T) {
		tk := mk("cap")
		many := make([]string, tasks.MaxStepsPerTask-2)
		for i := range many {
			many[i] = fmt.Sprintf("s%d", i)
		}
		add(tk, many...)
		_, err := tasks.AddSteps(ctx, pool, tk.ID, []string{"a", "b", "c"}, by)
		if err == nil || !strings.Contains(err.Error(), "2 more fit") {
			t.Fatalf("over the cap: %v, want an error saying 2 more fit", err)
		}
		add(tk, "a", "b")
	})

	t.Run("tick stamps and untick clears", func(t *testing.T) {
		tk := mk("tick")
		s := add(tk, "do it")[0]
		yes, no := true, false
		got, err := tasks.UpdateStep(ctx, pool, tk.ID, s.ID, tasks.UpdateStepParams{Done: &yes}, by)
		if err != nil || !got.Done || got.DoneAt == nil || got.DoneByUserID == nil || *got.DoneByUserID != u.ID {
			t.Fatalf("tick: %+v %v", got, err)
		}
		text := "did it"
		got, err = tasks.UpdateStep(ctx, pool, tk.ID, s.ID, tasks.UpdateStepParams{Text: &text}, by)
		if err != nil || got.Text != "did it" || !got.Done {
			t.Fatalf("rename keeps done: %+v %v", got, err)
		}
		got, err = tasks.UpdateStep(ctx, pool, tk.ID, s.ID, tasks.UpdateStepParams{Done: &no}, by)
		if err != nil || got.Done || got.DoneAt != nil || got.DoneByUserID != nil {
			t.Fatalf("untick: %+v %v", got, err)
		}
		if _, err := tasks.UpdateStep(ctx, pool, tk.ID, s.ID, tasks.UpdateStepParams{}, by); err == nil {
			t.Error("empty update succeeded")
		}
	})

	t.Run("remove and move keep positions dense", func(t *testing.T) {
		tk := mk("dense")
		s := add(tk, "a", "b", "c", "d", "e")
		if err := tasks.RemoveStep(ctx, pool, tk.ID, s[1].ID); err != nil {
			t.Fatal(err)
		}
		got, err := tasks.MoveStep(ctx, pool, tk.ID, s[4].ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"e", "a", "c", "d"}; !reflect.DeepEqual(stepTexts(got), want) {
			t.Errorf("after remove b, move e first: %v, want %v", stepTexts(got), want)
		}
		got, err = tasks.MoveStep(ctx, pool, tk.ID, s[0].ID, 99)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"e", "c", "d", "a"}; !reflect.DeepEqual(stepTexts(got), want) {
			t.Errorf("move a past the end: %v, want %v", stepTexts(got), want)
		}
	})

	t.Run("a step of another task is not found", func(t *testing.T) {
		a, b := mk("A"), mk("B")
		s := add(a, "mine")[0]
		yes := true
		if _, err := tasks.UpdateStep(ctx, pool, b.ID, s.ID, tasks.UpdateStepParams{Done: &yes}, by); !errors.Is(err, tasks.ErrStepNotFound) {
			t.Errorf("update via B: %v", err)
		}
		if err := tasks.RemoveStep(ctx, pool, b.ID, s.ID); !errors.Is(err, tasks.ErrStepNotFound) {
			t.Errorf("remove via B: %v", err)
		}
		if _, err := tasks.MoveStep(ctx, pool, b.ID, s.ID, 0); !errors.Is(err, tasks.ErrStepNotFound) {
			t.Errorf("move via B: %v", err)
		}
		if got, _ := tasks.ListSteps(ctx, pool, a.ID); len(got) != 1 || got[0].Done {
			t.Errorf("A's step changed: %+v", got)
		}
	})

	t.Run("reads carry progress", func(t *testing.T) {
		tk := mk("progress")
		bare := mk("no steps")
		s := add(tk, "a", "b", "c")
		yes := true
		if _, err := tasks.UpdateStep(ctx, pool, tk.ID, s[0].ID, tasks.UpdateStepParams{Done: &yes}, by); err != nil {
			t.Fatal(err)
		}
		got, err := tasks.Get(ctx, pool, tk.ID)
		if err != nil || got.Steps == nil || *got.Steps != (tasks.StepProgress{Done: 1, Total: 3}) {
			t.Fatalf("Get progress: %+v %v", got.Steps, err)
		}
		list, err := tasks.List(ctx, pool, tasks.ListFilter{ProjectID: p.ID})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range list {
			switch row.ID {
			case tk.ID:
				if row.Steps == nil || row.Steps.Total != 3 || row.Steps.Done != 1 {
					t.Errorf("list row progress = %+v", row.Steps)
				}
			case bare.ID:
				if row.Steps != nil {
					t.Errorf("task without steps has progress %+v", row.Steps)
				}
			}
		}
	})

	t.Run("close with pending steps closes and lists them", func(t *testing.T) {
		tk := mk("close")
		s := add(tk, "done one", "left one")
		yes := true
		if _, err := tasks.UpdateStep(ctx, pool, tk.ID, s[0].ID, tasks.UpdateStepParams{Done: &yes}, by); err != nil {
			t.Fatal(err)
		}
		res, err := tasks.Close(ctx, pool, tk.ID, tasks.CloseParams{State: tasks.StateDone}, by)
		if err != nil {
			t.Fatal(err)
		}
		if res.Task.State != tasks.StateDone {
			t.Errorf("state = %s", res.Task.State)
		}
		if want := []string{"left one"}; len(res.PendingSteps) != 1 || res.PendingSteps[0].Text != want[0] {
			t.Errorf("pending = %+v, want %v", res.PendingSteps, want)
		}
		clean := mk("clean close")
		res, err = tasks.Close(ctx, pool, clean.ID, tasks.CloseParams{State: tasks.StateDone}, by)
		if err != nil || res.PendingSteps == nil || len(res.PendingSteps) != 0 {
			t.Errorf("close without steps: pending %v, err %v", res.PendingSteps, err)
		}
	})

	t.Run("concurrent adds keep positions unique", func(t *testing.T) {
		tk := mk("race")
		var wg sync.WaitGroup
		errs := make(chan error, 10)
		for i := range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := tasks.AddSteps(ctx, pool, tk.ID, []string{fmt.Sprintf("g%d-a", i), fmt.Sprintf("g%d-b", i)}, by)
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		got, err := tasks.ListSteps(ctx, pool, tk.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 20 {
			t.Fatalf("got %d steps, want 20", len(got))
		}
		for i, s := range got {
			if s.Position != i {
				t.Fatalf("position %d at index %d", s.Position, i)
			}
		}
	})
}
