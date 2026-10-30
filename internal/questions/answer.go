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
	// ErrNotRecipient is returned when someone other than the person the
	// question is addressed to tries to answer or edit the answer.
	ErrNotRecipient = errors.New("only the person this question is addressed to can answer it")
	// ErrNotAsker is returned when someone other than the user who
	// asked tries to withdraw a question.
	ErrNotAsker = errors.New("only the user who asked this question can withdraw it")
	// ErrNotAskerRecord is returned when someone other than the user who
	// asked tries to record an answer given outside Nottario.
	ErrNotAskerRecord = errors.New("only the user who asked this question can record its answer")
	// ErrAlreadyAnswered is returned when recording an answer for a
	// question that already has one.
	ErrAlreadyAnswered = errors.New("already answered; nothing to record")
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

// Answer records the answer of userID, who must be the question's
// recipient, or replaces it while the asking
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
	// Checked before the state, so a non-recipient learns nothing about
	// the question beyond what the project already shows them.
	if cur.RecipientUserID == nil || *cur.RecipientUserID != userID {
		return nil, ErrNotRecipient
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

// ParseRecordChannel reads the channel of a recorded answer: "chat"
// (the default when blank) or "other". "web" is not one: answers given
// in the web UI go through Answer.
func ParseRecordChannel(s string) (Channel, error) {
	switch c := Channel(strings.ToLower(strings.TrimSpace(s))); c {
	case "":
		return ChannelChat, nil
	case ChannelChat, ChannelOther:
		return c, nil
	}
	return "", errInvalid(fmt.Sprintf("unknown channel %q; valid: chat, other", s))
}

// RecordAnswer records an answer the human gave the asking agent
// outside Nottario (in the chat, say), so the question reads answered
// here too. Only the user who asked may record it: the agent acts for
// that same human. The answer is attributed to the recipient, who gave
// it, through tokenID on channel, and is delivered at once: the agent
// already has it, so Take does not hand it over again. The question
// must be open. The row is locked like in Answer.
func RecordAnswer(ctx context.Context, pool *pgxpool.Pool, projectID, id, callerUserID uuid.UUID, tokenID *uuid.UUID, channel Channel, a Reply) (*Question, error) {
	if channel == "" {
		channel = ChannelChat
	}
	if channel != ChannelChat && channel != ChannelOther {
		return nil, errInvalid(fmt.Sprintf("unknown channel %q; valid: chat, other", channel))
	}
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
	if cur.AskedByUserID == nil || *cur.AskedByUserID != callerUserID {
		return nil, ErrNotAskerRecord
	}
	switch cur.State {
	case StateWithdrawn:
		return nil, ErrClosed
	case StateAnswered:
		return nil, ErrAlreadyAnswered
	}
	clean, err := normaliseAnswer(cur, a)
	if err != nil {
		return nil, err
	}
	row, err = q.RecordTaskAnswer(ctx, dbq.RecordTaskAnswerParams{
		Keys:    clean.Keys,
		Text:    clean.Text,
		Note:    clean.Note,
		UserID:  cur.RecipientUserID,
		TokenID: tokenID,
		Channel: string(channel),
		ID:      id,
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

// Withdraw takes back an open question on behalf of userID, who must
// be the user who asked it. A question already answered or withdrawn
// is not open any more and cannot be withdrawn. Closing a task
// withdraws its open questions in the tasks package, in the closing
// transaction.
func Withdraw(ctx context.Context, db dbq.DBTX, projectID, id, userID uuid.UUID, reason string) (*Question, error) {
	q := dbq.New(db)
	row, err := q.WithdrawTaskQuestion(ctx, dbq.WithdrawTaskQuestionParams{
		Reason: clampReason(reason), ID: id, ProjectID: projectID, UserID: userID,
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
	switch {
	case cur.AskedByUserID == nil || *cur.AskedByUserID != userID:
		return nil, ErrNotAsker
	case State(cur.State) == StateWithdrawn:
		return nil, ErrClosed
	}
	return nil, errInvalid("the question is already answered; only open questions can be withdrawn")
}

// PendingForTask returns the task's questions an agent still has to act
// on: open ones and answers not taken yet, oldest first. Never nil.
func PendingForTask(ctx context.Context, db dbq.DBTX, projectID, taskID uuid.UUID) ([]Question, error) {
	rows, err := dbq.New(db).ListPendingTaskQuestions(ctx, dbq.ListPendingTaskQuestionsParams{
		TaskID: taskID, ProjectID: projectID,
	})
	if err != nil {
		return nil, err
	}
	return fromRows(rows)
}

// ListFilter restricts a List call. ProjectIDs is required: it scopes
// the result to the projects the caller can see.
type ListFilter struct {
	ProjectIDs []uuid.UUID
	State      *State
	TaskID     *uuid.UUID
	SessionID  *uuid.UUID
	// RecipientUserID keeps the questions addressed to this user.
	RecipientUserID *uuid.UUID
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
		ProjectIds:      f.ProjectIDs,
		State:           state,
		TaskID:          f.TaskID,
		SessionID:       f.SessionID,
		RecipientUserID: f.RecipientUserID,
		Lim:             int32(lim), //nolint:gosec // clamped to MaxListLimit
	})
	if err != nil {
		return nil, err
	}
	return fromRows(rows)
}
