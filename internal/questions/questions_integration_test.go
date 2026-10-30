package questions_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/questions"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

func TestQuestions(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := t.Context()
	u, _, err := identity.UpsertFromGithub(ctx, pool, 9411, "questions", "Questions", "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := identity.CreateProject(ctx, pool, "QuestionsProj", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := identity.CreateProject(ctx, pool, "QuestionsOther", "", "", "", u.ID)
	if err != nil {
		t.Fatal(err)
	}
	by := tasks.Authorship{UserID: &u.ID}
	session := func(key string) uuid.UUID {
		t.Helper()
		s, err := agents.Touch(ctx, pool, agents.Ref{ProjectID: p.ID, UserID: u.ID, Key: key})
		if err != nil {
			t.Fatal(err)
		}
		return s.ID
	}
	mkTask := func(projectID uuid.UUID, title string) *tasks.Task {
		t.Helper()
		tk, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: projectID, Type: tasks.TypeTask, Title: title}, by)
		if err != nil {
			t.Fatal(err)
		}
		return tk
	}
	task := mkTask(p.ID, "questions host")
	ask := func(sid uuid.UUID, body string, opts ...string) *questions.Question {
		t.Helper()
		ap := questions.AskParams{ProjectID: p.ID, TaskID: task.ID, SessionID: &sid, AskedBy: by, Body: body}
		for _, o := range opts {
			ap.Options = append(ap.Options, questions.Option{Label: o})
		}
		q, err := questions.Ask(ctx, pool, ap)
		if err != nil {
			t.Fatal(err)
		}
		return q
	}
	options := func(labels ...string) []questions.Option {
		out := make([]questions.Option, 0, len(labels))
		for _, l := range labels {
			out = append(out, questions.Option{Label: l})
		}
		return out
	}

	t.Run("ask text", func(t *testing.T) {
		q := ask(session("s-text"), "  Which port?  ")
		if q.Kind != questions.KindText || q.Body != "Which port?" || q.State != questions.StateOpen ||
			q.Options != nil || q.Answer != nil || q.RecipientUserID == nil || *q.RecipientUserID != u.ID {
			t.Errorf("text question: %+v", q)
		}
		got, err := tasks.Get(ctx, pool, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.QuestionsOpen < 1 {
			t.Errorf("QuestionsOpen = %d, want at least 1", got.QuestionsOpen)
		}
	})

	t.Run("ask choice assigns keys and keeps the recommendation", func(t *testing.T) {
		sid := session("s-choice")
		q, err := questions.Ask(ctx, pool, questions.AskParams{
			ProjectID: p.ID, TaskID: task.ID, SessionID: &sid, AskedBy: by, Body: "Which db?",
			Options:     []questions.Option{{Label: " Postgres ", Description: "the usual"}, {Label: "SQLite"}, {Label: "MySQL"}},
			Recommended: 1, RecommendedReason: "already in compose",
		})
		if err != nil {
			t.Fatal(err)
		}
		if q.Kind != questions.KindChoice || len(q.Options) != 3 {
			t.Fatalf("choice question: %+v", q)
		}
		for i, o := range q.Options {
			if o.Key != []string{"1", "2", "3"}[i] {
				t.Errorf("option %d key = %q", i, o.Key)
			}
		}
		if q.Options[0].Label != "Postgres" || q.Options[0].Description != "the usual" ||
			q.RecommendedKey != "1" || q.RecommendedReason != "already in compose" {
			t.Errorf("choice question: %+v", q)
		}
	})

	t.Run("invalid asks", func(t *testing.T) {
		closed := mkTask(p.ID, "closed host")
		if _, _, err := tasks.SetState(ctx, pool, closed.ID, tasks.StateWontDo, &u.ID); err != nil {
			t.Fatal(err)
		}
		otherTask := mkTask(p.ID, "step owner")
		st, err := tasks.AddSteps(ctx, pool, otherTask.ID, []string{"foreign step"}, by)
		if err != nil {
			t.Fatal(err)
		}
		foreign := mkTask(other.ID, "other project task")
		ten := make([]string, 10)
		for i := range ten {
			ten[i] = string(rune('a' + i))
		}
		base := func() questions.AskParams {
			return questions.AskParams{ProjectID: p.ID, TaskID: task.ID, AskedBy: by, Body: "q?"}
		}
		cases := []struct {
			name  string
			mod   func(*questions.AskParams)
			check func(error) bool
		}{
			{"empty body", func(a *questions.AskParams) { a.Body = "  " }, questions.IsInvalid},
			{"one option", func(a *questions.AskParams) { a.Options = options("a") }, questions.IsInvalid},
			{"ten options", func(a *questions.AskParams) { a.Options = options(ten...) }, questions.IsInvalid},
			{"duplicate labels", func(a *questions.AskParams) { a.Options = options("Yes", "yes") }, questions.IsInvalid},
			{"empty label", func(a *questions.AskParams) { a.Options = options("a", " ") }, questions.IsInvalid},
			{"recommended out of range", func(a *questions.AskParams) { a.Options = options("a", "b"); a.Recommended = 3 }, questions.IsInvalid},
			{"recommended without options", func(a *questions.AskParams) { a.Recommended = 1 }, questions.IsInvalid},
			{"reason without recommendation", func(a *questions.AskParams) { a.Options = options("a", "b"); a.RecommendedReason = "why" }, questions.IsInvalid},
			{"multi without options", func(a *questions.AskParams) { a.Multi = true }, questions.IsInvalid},
			{"closed task", func(a *questions.AskParams) { a.TaskID = closed.ID }, questions.IsInvalid},
			{"step of another task", func(a *questions.AskParams) { a.StepID = &st[0].ID }, func(err error) bool { return errors.Is(err, tasks.ErrStepNotFound) }},
			{"task of another project", func(a *questions.AskParams) { a.TaskID = foreign.ID }, func(err error) bool { return errors.Is(err, tasks.ErrNotFound) }},
		}
		for _, c := range cases {
			a := base()
			c.mod(&a)
			if _, err := questions.Ask(ctx, pool, a); err == nil || !c.check(err) {
				t.Errorf("%s: err = %v", c.name, err)
			}
		}
	})

	t.Run("answer text", func(t *testing.T) {
		q := ask(session("s-ans-text"), "Name?")
		if _, err := questions.Answer(ctx, pool, p.ID, q.ID, u.ID, questions.Reply{Keys: []string{"1"}, Text: "x"}); !questions.IsInvalid(err) {
			t.Errorf("keys on a text question: err = %v", err)
		}
		if _, err := questions.Answer(ctx, pool, p.ID, q.ID, u.ID, questions.Reply{Text: "  "}); !questions.IsInvalid(err) {
			t.Errorf("empty text: err = %v", err)
		}
		got, err := questions.Answer(ctx, pool, p.ID, q.ID, u.ID, questions.Reply{Text: " nottario ", Note: "lowercase"})
		if err != nil {
			t.Fatal(err)
		}
		if got.State != questions.StateAnswered || got.Answer == nil || got.Answer.Text != "nottario" ||
			got.Answer.Note != "lowercase" || got.AnsweredByUserID == nil || got.AnsweredAt == nil {
			t.Errorf("answered: %+v", got)
		}
		if _, err := questions.Answer(ctx, pool, other.ID, q.ID, u.ID, questions.Reply{Text: "x"}); !errors.Is(err, questions.ErrNotFound) {
			t.Errorf("answer through another project: err = %v", err)
		}
	})

	t.Run("only the recipient answers", func(t *testing.T) {
		q := ask(session("s-recipient"), "Whose call?")
		intruder, _, err := identity.UpsertFromGithub(ctx, pool, 15990, "q-intruder", "Intruder", "")
		if err != nil {
			t.Fatal(err)
		}
		roles, err := identity.ListRoles(ctx, pool, p.ID)
		if err != nil || len(roles) == 0 {
			t.Fatalf("roles: %v %v", roles, err)
		}
		if err := identity.AddMembership(ctx, pool, intruder.ID, p.ID, roles[0].ID); err != nil {
			t.Fatal(err)
		}
		if _, err := questions.Answer(ctx, pool, p.ID, q.ID, intruder.ID, questions.Reply{Text: "mine"}); !errors.Is(err, questions.ErrNotRecipient) {
			t.Errorf("answer by a member who is not the recipient: err = %v", err)
		}
		if _, err := questions.Answer(ctx, pool, p.ID, q.ID, u.ID, questions.Reply{Text: "ours"}); err != nil {
			t.Fatal(err)
		}
		if _, err := questions.Answer(ctx, pool, p.ID, q.ID, intruder.ID, questions.Reply{Text: "edit"}); !errors.Is(err, questions.ErrNotRecipient) {
			t.Errorf("edit by a member who is not the recipient: err = %v", err)
		}
	})

	t.Run("answer choice", func(t *testing.T) {
		q := ask(session("s-ans-choice"), "Pick", "A", "B", "C")
		bad := []questions.Reply{
			{Keys: []string{"4"}},
			{Keys: []string{"1", "2"}},
			{},
		}
		for _, r := range bad {
			if _, err := questions.Answer(ctx, pool, p.ID, q.ID, u.ID, r); !questions.IsInvalid(err) {
				t.Errorf("answer %+v: err = %v", r, err)
			}
		}
		got, err := questions.Answer(ctx, pool, p.ID, q.ID, u.ID, questions.Reply{Text: "none of them"})
		if err != nil {
			t.Fatalf("Other text: %v", err)
		}
		if len(got.Answer.Keys) != 0 || got.Answer.Text != "none of them" {
			t.Errorf("Other answer: %+v", got.Answer)
		}

		sid := session("s-ans-multi")
		mq, err := questions.Ask(ctx, pool, questions.AskParams{
			ProjectID: p.ID, TaskID: task.ID, SessionID: &sid, AskedBy: by, Body: "Pick some",
			Options: options("A", "B", "C"), Multi: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := questions.Answer(ctx, pool, p.ID, mq.ID, u.ID, questions.Reply{Keys: []string{"1", "1"}}); !questions.IsInvalid(err) {
			t.Errorf("duplicate key: err = %v", err)
		}
		got, err = questions.Answer(ctx, pool, p.ID, mq.ID, u.ID, questions.Reply{Keys: []string{"1", "3"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Answer.Keys) != 2 {
			t.Errorf("multi answer: %+v", got.Answer)
		}
	})

	t.Run("edit before take, take once, edit after take fails", func(t *testing.T) {
		sid := session("s-take")
		q := ask(sid, "Go?", "Yes", "No")
		ask(sid, "Still open?") // counted by Take as open
		if _, err := questions.Answer(ctx, pool, p.ID, q.ID, u.ID, questions.Reply{Keys: []string{"1"}}); err != nil {
			t.Fatal(err)
		}
		if _, err := questions.Answer(ctx, pool, p.ID, q.ID, u.ID, questions.Reply{Keys: []string{"2"}}); err != nil {
			t.Fatalf("edit before take: %v", err)
		}
		got, n, err := questions.Take(ctx, pool, p.ID, sid)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != q.ID || got[0].DeliveredAt == nil || got[0].Answer.Keys[0] != "2" || n != 1 {
			t.Fatalf("take: %+v open=%d", got, n)
		}
		again, n, err := questions.Take(ctx, pool, p.ID, sid)
		if err != nil {
			t.Fatal(err)
		}
		if len(again) != 0 || n != 1 {
			t.Errorf("second take: %+v open=%d", again, n)
		}
		if _, err := questions.Answer(ctx, pool, p.ID, q.ID, u.ID, questions.Reply{Keys: []string{"1"}}); !errors.Is(err, questions.ErrAlreadyDelivered) {
			t.Errorf("edit after take: err = %v", err)
		}
	})

	t.Run("concurrent takes hand the answer out once", func(t *testing.T) {
		for round := range 10 {
			sid := session("s-race-" + string(rune('a'+round)))
			q := ask(sid, "Race?")
			if _, err := questions.Answer(ctx, pool, p.ID, q.ID, u.ID, questions.Reply{Text: "go"}); err != nil {
				t.Fatal(err)
			}
			const takers = 8
			var wg sync.WaitGroup
			results := make([][]questions.Question, takers)
			errs := make([]error, takers)
			start := make(chan struct{})
			for i := range takers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					results[i], _, errs[i] = questions.Take(ctx, pool, p.ID, sid)
				}()
			}
			close(start)
			wg.Wait()
			seen := 0
			for i := range takers {
				if errs[i] != nil {
					t.Fatalf("take %d: %v", i, errs[i])
				}
				for _, r := range results[i] {
					if r.ID == q.ID {
						seen++
					}
				}
			}
			if seen != 1 {
				t.Fatalf("round %d: answer handed out %d times, want 1", round, seen)
			}
		}
	})

	t.Run("withdraw then answer", func(t *testing.T) {
		q := ask(session("s-withdraw"), "Withdraw me?")
		w, err := questions.Withdraw(ctx, pool, p.ID, q.ID, u.ID, "  not needed  ")
		if err != nil {
			t.Fatal(err)
		}
		if w.State != questions.StateWithdrawn || w.WithdrawnAt == nil || w.WithdrawnReason != "not needed" {
			t.Errorf("withdrawn: %+v", w)
		}
		if _, err := questions.Answer(ctx, pool, p.ID, q.ID, u.ID, questions.Reply{Text: "late"}); !errors.Is(err, questions.ErrClosed) {
			t.Errorf("answer after withdraw: err = %v", err)
		}
		if _, err := questions.Withdraw(ctx, pool, p.ID, q.ID, u.ID, ""); !errors.Is(err, questions.ErrClosed) {
			t.Errorf("withdraw twice: err = %v", err)
		}
		if _, err := questions.Withdraw(ctx, pool, p.ID, uuid.New(), u.ID, ""); !errors.Is(err, questions.ErrNotFound) {
			t.Errorf("withdraw unknown: err = %v", err)
		}
	})

	t.Run("only the asker withdraws", func(t *testing.T) {
		q := ask(session("s-asker"), "Mine to withdraw?")
		other, _, err := identity.UpsertFromGithub(ctx, pool, 9412, "not-asker", "Not asker", "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := questions.Withdraw(ctx, pool, p.ID, q.ID, other.ID, ""); !errors.Is(err, questions.ErrNotAsker) {
			t.Fatalf("withdraw by another user: %v", err)
		}
		if _, err := questions.Withdraw(ctx, pool, p.ID, q.ID, u.ID, ""); err != nil {
			t.Fatalf("withdraw by the asker: %v", err)
		}
	})

	t.Run("a parent closed by rollup withdraws its open questions", func(t *testing.T) {
		parent, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeFeature, Title: "rollup parent"}, by)
		if err != nil {
			t.Fatal(err)
		}
		child, err := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "rollup child", ParentTaskID: &parent.ID}, by)
		if err != nil {
			t.Fatal(err)
		}
		pq, err := questions.Ask(ctx, pool, questions.AskParams{ProjectID: p.ID, TaskID: parent.ID, AskedBy: by, Body: "parent question"})
		if err != nil {
			t.Fatal(err)
		}
		_, n, err := tasks.SetState(ctx, pool, child.ID, tasks.StateDone, &u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("child withdrew %d, want 0 (the count is the task's own)", n)
		}
		if got, err := tasks.Get(ctx, pool, parent.ID); err != nil || got.State != tasks.StateDone {
			t.Fatalf("parent after rollup: %+v, %v", got, err)
		}
		var state, reason string
		if err := pool.QueryRow(ctx, `SELECT state, withdrawn_reason FROM task_questions WHERE id = $1`, pq.ID).Scan(&state, &reason); err != nil {
			t.Fatal(err)
		}
		if state != "withdrawn" || reason != tasks.WithdrawReasonTaskClosed {
			t.Errorf("parent question = %s (%q), want withdrawn (task closed)", state, reason)
		}
	})

	t.Run("closing the task withdraws its open questions", func(t *testing.T) {
		host := mkTask(p.ID, "withdraw host")
		sid := session("s-wofor")
		for _, body := range []string{"one", "two", "three"} {
			if _, err := questions.Ask(ctx, pool, questions.AskParams{ProjectID: p.ID, TaskID: host.ID, SessionID: &sid, AskedBy: by, Body: body}); err != nil {
				t.Fatal(err)
			}
		}
		qs, err := questions.List(ctx, pool, questions.ListFilter{ProjectIDs: []uuid.UUID{p.ID}, TaskID: &host.ID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := questions.Answer(ctx, pool, p.ID, qs[0].ID, u.ID, questions.Reply{Text: "done"}); err != nil {
			t.Fatal(err)
		}
		_, n, err := tasks.SetState(ctx, pool, host.ID, tasks.StateWontDo, &u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Errorf("withdrew %d, want 2", n)
		}
		got, err := tasks.Get(ctx, pool, host.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.QuestionsOpen != 0 {
			t.Errorf("QuestionsOpen = %d after withdraw", got.QuestionsOpen)
		}
	})

	t.Run("list scoping", func(t *testing.T) {
		ft := mkTask(other.ID, "other host")
		if _, err := questions.Ask(ctx, pool, questions.AskParams{ProjectID: other.ID, TaskID: ft.ID, AskedBy: by, Body: "elsewhere"}); err != nil {
			t.Fatal(err)
		}
		mine, err := questions.List(ctx, pool, questions.ListFilter{ProjectIDs: []uuid.UUID{p.ID}})
		if err != nil {
			t.Fatal(err)
		}
		for _, q := range mine {
			if q.ProjectID != p.ID {
				t.Errorf("list leaked question of project %s", q.ProjectID)
			}
		}
		both, err := questions.List(ctx, pool, questions.ListFilter{ProjectIDs: []uuid.UUID{p.ID, other.ID}})
		if err != nil {
			t.Fatal(err)
		}
		if len(both) != len(mine)+1 {
			t.Errorf("both projects: %d, want %d", len(both), len(mine)+1)
		}
		openState := questions.StateOpen
		open, err := questions.List(ctx, pool, questions.ListFilter{ProjectIDs: []uuid.UUID{p.ID}, State: &openState})
		if err != nil {
			t.Fatal(err)
		}
		for i, q := range open {
			if q.State != questions.StateOpen {
				t.Errorf("state filter let through %s", q.State)
			}
			if i > 0 && q.CreatedAt.Before(open[i-1].CreatedAt) {
				t.Errorf("open questions not oldest first")
			}
		}
		if len(mine) > 0 && mine[0].State != questions.StateOpen {
			t.Errorf("open questions do not come first: %s", mine[0].State)
		}
		none, err := questions.List(ctx, pool, questions.ListFilter{})
		if err != nil || len(none) != 0 {
			t.Errorf("no project ids: %v, %d rows", err, len(none))
		}
	})
	t.Run("record answer given in chat", func(t *testing.T) {
		_, tok, err := identity.IssueToken(ctx, pool, u.ID, p.ID, "recorder", nil)
		if err != nil {
			t.Fatal(err)
		}
		sid := session("s-record")
		q := ask(sid, "Postgres or SQLite?", "Postgres", "SQLite")
		got, err := questions.RecordAnswer(ctx, pool, p.ID, q.ID, u.ID, &tok.ID, "", questions.Reply{Keys: []string{"2"}})
		if err != nil {
			t.Fatalf("RecordAnswer: %v", err)
		}
		if got.State != questions.StateAnswered || got.AnswerChannel != questions.ChannelChat ||
			got.AnsweredByTokenID == nil || *got.AnsweredByTokenID != tok.ID ||
			got.AnsweredByUserID == nil || *got.AnsweredByUserID != u.ID ||
			got.AnsweredAt == nil || got.DeliveredAt == nil || got.Answer == nil || got.Answer.Keys[0] != "2" {
			t.Errorf("recorded: %+v", got)
		}
		taken, open, err := questions.Take(ctx, pool, p.ID, sid)
		if err != nil {
			t.Fatal(err)
		}
		if len(taken) != 0 || open != 0 {
			t.Errorf("take after record = %d answers, %d open", len(taken), open)
		}
		// Recording twice, or over a web answer, is refused.
		if _, err := questions.RecordAnswer(ctx, pool, p.ID, q.ID, u.ID, &tok.ID, questions.ChannelChat, questions.Reply{Text: "again"}); !errors.Is(err, questions.ErrAlreadyAnswered) {
			t.Errorf("record twice: %v", err)
		}
		web := ask(sid, "Which port?")
		if _, err := questions.Answer(ctx, pool, p.ID, web.ID, u.ID, questions.Reply{Text: "8080"}); err != nil {
			t.Fatal(err)
		}
		if _, err := questions.RecordAnswer(ctx, pool, p.ID, web.ID, u.ID, &tok.ID, questions.ChannelChat, questions.Reply{Text: "9090"}); !errors.Is(err, questions.ErrAlreadyAnswered) {
			t.Errorf("record over a web answer: %v", err)
		}

		// A choice question also takes the human's own wording; channel other.
		own := ask(sid, "Which cache?", "Redis", "None")
		got, err = questions.RecordAnswer(ctx, pool, p.ID, own.ID, u.ID, nil, questions.ChannelOther, questions.Reply{Text: "memcached, we have it"})
		if err != nil || got.AnswerChannel != questions.ChannelOther || got.Answer.Text != "memcached, we have it" {
			t.Errorf("record own wording: %+v err=%v", got, err)
		}

		// Invalid replies and channels are refused and leave it open.
		bad := ask(sid, "One or two?", "One", "Two")
		for name, r := range map[string]questions.Reply{
			"empty":       {},
			"unknown key": {Keys: []string{"7"}},
			"two keys":    {Keys: []string{"1", "2"}},
		} {
			if _, err := questions.RecordAnswer(ctx, pool, p.ID, bad.ID, u.ID, &tok.ID, questions.ChannelChat, r); !questions.IsInvalid(err) {
				t.Errorf("%s: %v", name, err)
			}
		}
		if _, err := questions.RecordAnswer(ctx, pool, p.ID, bad.ID, u.ID, &tok.ID, questions.ChannelWeb, questions.Reply{Keys: []string{"1"}}); !questions.IsInvalid(err) {
			t.Errorf("channel web: %v", err)
		}

		// Only the asker may record.
		stranger, _, err := identity.UpsertFromGithub(ctx, pool, 9412, "questions-stranger", "Stranger", "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := questions.RecordAnswer(ctx, pool, p.ID, bad.ID, stranger.ID, nil, questions.ChannelChat, questions.Reply{Keys: []string{"1"}}); !errors.Is(err, questions.ErrNotAskerRecord) {
			t.Errorf("non-asker: %v", err)
		}

		// A withdrawn question is closed.
		if _, err := questions.Withdraw(ctx, pool, p.ID, bad.ID, u.ID, "moot"); err != nil {
			t.Fatal(err)
		}
		if _, err := questions.RecordAnswer(ctx, pool, p.ID, bad.ID, u.ID, &tok.ID, questions.ChannelChat, questions.Reply{Keys: []string{"1"}}); !errors.Is(err, questions.ErrClosed) {
			t.Errorf("withdrawn: %v", err)
		}
		if _, err := questions.RecordAnswer(ctx, pool, p.ID, uuid.New(), u.ID, nil, questions.ChannelChat, questions.Reply{Text: "x"}); !errors.Is(err, questions.ErrNotFound) {
			t.Errorf("unknown id: %v", err)
		}
	})
}
