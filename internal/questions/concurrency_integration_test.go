package questions_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/questions"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// storm runs every fn at once: each goroutine waits on one release
// channel so the calls overlap as much as possible.
func storm(fns ...func()) {
	ready := &sync.WaitGroup{}
	ready.Add(len(fns))
	release := make(chan struct{})
	done := &sync.WaitGroup{}
	done.Add(len(fns))
	for _, fn := range fns {
		go func() {
			defer done.Done()
			ready.Done()
			<-release
			fn()
		}()
	}
	ready.Wait()
	close(release)
	done.Wait()
}

func isDeadlock(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "40P01"
}

// errSink collects the errors of a storm that are neither nil nor
// expected.
type errSink struct {
	mu   sync.Mutex
	errs []string
}

func (s *errSink) add(who string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kind := "unexpected"
	if isDeadlock(err) {
		kind = "DEADLOCK"
	}
	s.errs = append(s.errs, fmt.Sprintf("%s: %s: %v", kind, who, err))
}

// Closing a task and answering its questions take locks in opposite
// orders (task then questions, question then the task's activity row);
// the activity trigger never waits, so neither side can deadlock.
func TestQuestionsConcurrency(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 9421, "q-racer", "Racer", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "QuestionsRace", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	by := tasks.Authorship{UserID: &u.ID}
	s, err := agents.Touch(ctx, pool, agents.Ref{ProjectID: p.ID, UserID: u.ID, Key: "racer"})
	if err != nil {
		t.Fatal(err)
	}
	const rounds, workers = 8, 8
	mkTask := func(title string) *tasks.Task {
		t.Helper()
		tk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: title}, by)
		if err != nil {
			t.Fatal(err)
		}
		return tk
	}

	t.Run("answers racing a close", func(t *testing.T) {
		var sink errSink
		for r := range rounds {
			tk := mkTask(fmt.Sprintf("answer race %d", r))
			ids := make([]uuid.UUID, workers)
			for i := range ids {
				q, err := questions.Ask(ctx, pool, questions.AskParams{
					ProjectID: p.ID, TaskID: tk.ID, SessionID: &s.ID, AskedBy: by, Body: fmt.Sprintf("q%d", i),
				})
				if err != nil {
					t.Fatal(err)
				}
				ids[i] = q.ID
			}
			fns := []func(){func() {
				if _, err := tasks.Close(ctx, pool, tk.ID, tasks.CloseParams{State: tasks.StateDone}, by); err != nil {
					sink.add("close", err)
				}
			}}
			for _, id := range ids {
				fns = append(fns, func() {
					_, err := questions.Answer(ctx, pool, p.ID, id, u.ID, questions.Reply{Text: "yes"})
					if err != nil && !errors.Is(err, questions.ErrClosed) {
						sink.add("answer", err)
					}
				})
			}
			storm(fns...)
			var open int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM task_questions WHERE task_id = $1 AND state = 'open'`, tk.ID).Scan(&open); err != nil {
				t.Fatal(err)
			}
			if open != 0 {
				t.Errorf("round %d: %d questions still open after the close", r, open)
			}
		}
		for _, e := range sink.errs {
			t.Error(e)
		}
	})

	t.Run("asks racing a close with a comment", func(t *testing.T) {
		var sink errSink
		for r := range rounds {
			tk := mkTask(fmt.Sprintf("ask race %d", r))
			fns := []func(){func() {
				_, err := tasks.Close(ctx, pool, tk.ID, tasks.CloseParams{State: tasks.StateDone, Comment: "closing"}, by)
				if err != nil {
					sink.add("close", err)
				}
			}}
			for i := range workers {
				fns = append(fns, func() {
					_, err := questions.Ask(ctx, pool, questions.AskParams{
						ProjectID: p.ID, TaskID: tk.ID, SessionID: &s.ID, AskedBy: by, Body: fmt.Sprintf("late %d", i),
					})
					// A question after the close is refused as invalid.
					if err != nil && !questions.IsInvalid(err) {
						sink.add("ask", err)
					}
				})
			}
			storm(fns...)
			var open int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM task_questions WHERE task_id = $1 AND state = 'open'`, tk.ID).Scan(&open); err != nil {
				t.Fatal(err)
			}
			if open != 0 {
				t.Errorf("round %d: %d questions open on a closed task", r, open)
			}
		}
		for _, e := range sink.errs {
			t.Error(e)
		}
	})
}
