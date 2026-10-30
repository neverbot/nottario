package questions

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
)

var (
	// ErrNotFound is returned when a question id does not exist or
	// belongs to another project. Both read the same.
	ErrNotFound = errors.New("question not found")
	// ErrClosed is returned when answering a withdrawn question.
	ErrClosed = errors.New("question is withdrawn")
	// ErrAlreadyDelivered is returned when editing an answer the asking
	// session has already taken.
	ErrAlreadyDelivered = errors.New("the agent already picked up this answer; it can no longer change")
)

// normaliseAnswer trims an answer and checks it fits the question.
func normaliseAnswer(q *Question, a Reply) (Reply, error) {
	out := Reply{
		Keys: make([]string, 0, len(a.Keys)),
		Text: strings.TrimSpace(a.Text),
		Note: strings.TrimSpace(a.Note),
	}
	if runeLen(out.Text) > MaxAnswerLen {
		return Reply{}, errInvalid(fmt.Sprintf("answer text is longer than %d characters", MaxAnswerLen))
	}
	if runeLen(out.Note) > MaxAnswerLen {
		return Reply{}, errInvalid(fmt.Sprintf("answer note is longer than %d characters", MaxAnswerLen))
	}
	for _, k := range a.Keys {
		if k = strings.TrimSpace(k); k != "" {
			out.Keys = append(out.Keys, k)
		}
	}
	if q.Kind == KindText {
		if len(out.Keys) > 0 {
			return Reply{}, errInvalid("this question takes a text answer, not options")
		}
		if out.Text == "" {
			return Reply{}, errInvalid("answer text is empty")
		}
		return out, nil
	}
	if len(out.Keys) == 0 && out.Text == "" {
		return Reply{}, errInvalid("pick an option or write an answer")
	}
	if len(out.Keys) > 1 && !q.Multi {
		return Reply{}, errInvalid("this question takes one option")
	}
	valid := make(map[string]bool, len(q.Options))
	for _, o := range q.Options {
		valid[o.Key] = true
	}
	seen := make(map[string]bool, len(out.Keys))
	for _, k := range out.Keys {
		if !valid[k] {
			return Reply{}, errInvalid(fmt.Sprintf("%q is not one of the options", k))
		}
		if seen[k] {
			return Reply{}, errInvalid(fmt.Sprintf("option %s given twice", k))
		}
		seen[k] = true
	}
	return out, nil
}

// Answer records the human's answer, or replaces it while the asking
// agent has not taken it yet. The question row is locked for the whole
// check-and-write, so an edit and a Take cannot interleave.
func Answer(ctx context.Context, pool *pgxpool.Pool, projectID, id, userID uuid.UUID, a Reply) (*Question, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)
	row, err := q.GetTaskQuestionForUpdate(ctx, dbq.GetTaskQuestionForUpdateParams{ID: id, ProjectID: projectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	cur, err := fromRow(row)
	if err != nil {
		return nil, err
	}
	switch {
	case cur.State == StateWithdrawn:
		return nil, ErrClosed
	case cur.DeliveredAt != nil:
		return nil, ErrAlreadyDelivered
	}
	clean, err := normaliseAnswer(cur, a)
	if err != nil {
		return nil, err
	}
	row, err = q.AnswerTaskQuestion(ctx, dbq.AnswerTaskQuestionParams{
		Keys:   clean.Keys,
		Text:   clean.Text,
		Note:   clean.Note,
		UserID: userID,
		ID:     id,
	})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return fromRow(row)
}

// Take returns a session's answers it has not seen and marks them
// delivered, plus how many of its questions are still open. Each answer
// is handed to exactly one caller, however many take at once.
func Take(ctx context.Context, pool *pgxpool.Pool, projectID, sessionID uuid.UUID) ([]Question, int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)
	rows, err := q.TakeTaskAnswers(ctx, dbq.TakeTaskAnswersParams{ProjectID: projectID, SessionID: sessionID})
	if err != nil {
		return nil, 0, err
	}
	open, err := q.CountOpenQuestionsForSession(ctx, dbq.CountOpenQuestionsForSessionParams{ProjectID: projectID, SessionID: sessionID})
	if err != nil {
		return nil, 0, err
	}
	out, err := fromRows(rows)
	if err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return out, int(open), nil
}

// clampReason trims a withdraw reason to what the column takes.
func clampReason(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > MaxReasonLen {
		return string(r[:MaxReasonLen])
	}
	return s
}

// Withdraw takes back an open question. A question already answered or
// withdrawn is not open any more and cannot be withdrawn.
func Withdraw(ctx context.Context, db dbq.DBTX, projectID, id uuid.UUID, reason string) (*Question, error) {
	q := dbq.New(db)
	row, err := q.WithdrawTaskQuestion(ctx, dbq.WithdrawTaskQuestionParams{
		Reason: clampReason(reason), ID: id, ProjectID: projectID,
	})
	if err == nil {
		return fromRow(row)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	cur, err := q.GetTaskQuestion(ctx, dbq.GetTaskQuestionParams{ID: id, ProjectID: projectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if State(cur.State) == StateWithdrawn {
		return nil, ErrClosed
	}
	return nil, errInvalid("the question is already answered; only open questions can be withdrawn")
}

// WithdrawOpenForTask withdraws every open question of a task, for when
// the task closes, and returns how many it withdrew.
func WithdrawOpenForTask(ctx context.Context, db dbq.DBTX, taskID uuid.UUID, reason string) (int, error) {
	rows, err := dbq.New(db).WithdrawOpenTaskQuestions(ctx, dbq.WithdrawOpenTaskQuestionsParams{
		Reason: clampReason(reason), TaskID: taskID,
	})
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

// ListFilter restricts a List call. ProjectIDs is required: it scopes
// the result to the projects the caller can see.
type ListFilter struct {
	ProjectIDs []uuid.UUID
	State      *State
	TaskID     *uuid.UUID
	SessionID  *uuid.UUID
	// Limit defaults to DefaultListLimit and is capped at MaxListLimit.
	Limit int
}

// List bounds.
const (
	DefaultListLimit = 100
	MaxListLimit     = 500
)

// List returns questions matching the filter: open ones first, oldest
// first, then the rest, most recently changed first. Never nil.
func List(ctx context.Context, db dbq.DBTX, f ListFilter) ([]Question, error) {
	if len(f.ProjectIDs) == 0 {
		return []Question{}, nil
	}
	lim := f.Limit
	switch {
	case lim <= 0:
		lim = DefaultListLimit
	case lim > MaxListLimit:
		lim = MaxListLimit
	}
	var state pgtype.Text
	if f.State != nil {
		state = pgtype.Text{String: string(*f.State), Valid: true}
	}
	rows, err := dbq.New(db).ListTaskQuestions(ctx, dbq.ListTaskQuestionsParams{
		ProjectIds: f.ProjectIDs,
		State:      state,
		TaskID:     f.TaskID,
		SessionID:  f.SessionID,
		Lim:        int32(lim), //nolint:gosec // clamped to MaxListLimit
	})
	if err != nil {
		return nil, err
	}
	return fromRows(rows)
}
