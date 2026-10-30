// Package questions holds the questions agents ask the human: tied to
// a task (and maybe a step), answered in the web UI, collected by the
// asking session with Take.
package questions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/tasks"
)

// Kind is the shape of the answer a question expects.
type Kind string

const (
	KindText   Kind = "text"
	KindChoice Kind = "choice"
)

// State is where a question is in its life.
type State string

const (
	StateOpen      State = "open"
	StateAnswered  State = "answered"
	StateWithdrawn State = "withdrawn"
)

// Option is one choice of a choice question. Keys are "1" to "9", in
// the order the agent listed the options.
type Option struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Reply is what the human answered: option keys for a choice question
// (or an "Other" text), free text for a text one, and an optional note.
// (Named Reply so that Answer can be the function that records it.)
type Reply struct {
	Keys []string `json:"keys,omitempty"`
	Text string   `json:"text,omitempty"`
	Note string   `json:"note,omitempty"`
}

// Question is one question an agent asked about a task.
type Question struct {
	ID                uuid.UUID  `json:"id"`
	ProjectID         uuid.UUID  `json:"project_id"`
	TaskID            uuid.UUID  `json:"task_id"`
	StepID            *uuid.UUID `json:"step_id,omitempty"`
	SessionID         *uuid.UUID `json:"session_id,omitempty"`
	AskedByUserID     *uuid.UUID `json:"asked_by_user_id,omitempty"`
	RecipientUserID   *uuid.UUID `json:"recipient_user_id,omitempty"`
	AnsweredByUserID  *uuid.UUID `json:"answered_by_user_id,omitempty"`
	Kind              Kind       `json:"kind"`
	Body              string     `json:"body"`
	Options           []Option   `json:"options,omitempty"`
	Multi             bool       `json:"multi,omitempty"`
	RecommendedKey    string     `json:"recommended_key,omitempty"`
	RecommendedReason string     `json:"recommended_reason,omitempty"`
	State             State      `json:"state"`
	// Answer is nil until the question is answered.
	Answer          *Reply     `json:"answer,omitempty"`
	AnsweredAt      *time.Time `json:"answered_at,omitempty"`
	DeliveredAt     *time.Time `json:"delivered_at,omitempty"`
	WithdrawnAt     *time.Time `json:"withdrawn_at,omitempty"`
	WithdrawnReason string     `json:"withdrawn_reason,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Limits, in characters. The schema carries the same bounds as CHECKs.
const (
	MaxBodyLen        = 4000
	MaxOptions        = 9
	MaxOptionLabelLen = 120
	MaxOptionDescLen  = 300
	MaxReasonLen      = 300
	MaxAnswerLen      = 8000
)

type invalidErr string

func (e invalidErr) Error() string { return string(e) }

func errInvalid(msg string) error { return invalidErr(msg) }

// IsInvalid reports whether err is a rejected input rather than a
// failure, so handlers can answer 400 instead of 500.
func IsInvalid(err error) bool {
	var e invalidErr
	return errors.As(err, &e)
}

// AskParams carries a new question.
type AskParams struct {
	ProjectID, TaskID uuid.UUID
	StepID, SessionID *uuid.UUID
	// AskedBy is the asking user and token; the user is also the
	// recipient.
	AskedBy tasks.Authorship
	Body    string
	// Options are labels and descriptions; Ask assigns the keys. None
	// makes a text question.
	Options []Option
	// Recommended is the 1-based option the agent recommends; 0 is none.
	Recommended       int
	RecommendedReason string
	Multi             bool
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// normaliseOptions trims the options, checks them and assigns keys.
func normaliseOptions(in []Option) ([]Option, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) < 2 || len(in) > MaxOptions {
		return nil, errInvalid(fmt.Sprintf("a choice question needs 2 to %d options; ask without options for a free-text answer", MaxOptions))
	}
	out := make([]Option, 0, len(in))
	seen := make(map[string]int, len(in))
	for i, o := range in {
		label := strings.TrimSpace(o.Label)
		desc := strings.TrimSpace(o.Description)
		switch {
		case label == "":
			return nil, errInvalid(fmt.Sprintf("option %d: label is empty", i+1))
		case runeLen(label) > MaxOptionLabelLen:
			return nil, errInvalid(fmt.Sprintf("option %d: label is longer than %d characters", i+1, MaxOptionLabelLen))
		case runeLen(desc) > MaxOptionDescLen:
			return nil, errInvalid(fmt.Sprintf("option %d: description is longer than %d characters", i+1, MaxOptionDescLen))
		}
		fold := strings.ToLower(label)
		if j, dup := seen[fold]; dup {
			return nil, errInvalid(fmt.Sprintf("options %d and %d have the same label", j, i+1))
		}
		seen[fold] = i + 1
		out = append(out, Option{Key: strconv.Itoa(i + 1), Label: label, Description: desc})
	}
	return out, nil
}

// Ask files a question on a task. The task must belong to the project
// and be open; the step, when given, must be one of the task's.
func Ask(ctx context.Context, pool *pgxpool.Pool, p AskParams) (*Question, error) {
	body := strings.TrimSpace(p.Body)
	if body == "" {
		return nil, errInvalid("question is empty")
	}
	if runeLen(body) > MaxBodyLen {
		return nil, errInvalid(fmt.Sprintf("question is longer than %d characters", MaxBodyLen))
	}
	opts, err := normaliseOptions(p.Options)
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(p.RecommendedReason)
	switch {
	case p.Recommended != 0 && len(opts) == 0:
		return nil, errInvalid("recommended needs options")
	case p.Recommended < 0 || p.Recommended > len(opts):
		return nil, errInvalid(fmt.Sprintf("recommended must be an option number between 1 and %d", len(opts)))
	case reason != "" && p.Recommended == 0:
		return nil, errInvalid("recommended_reason needs recommended")
	case runeLen(reason) > MaxReasonLen:
		return nil, errInvalid(fmt.Sprintf("recommended_reason is longer than %d characters", MaxReasonLen))
	case p.Multi && len(opts) == 0:
		return nil, errInvalid("multi needs options")
	}

	kind := KindText
	var optsJSON []byte
	var recKey pgtype.Text
	if len(opts) > 0 {
		kind = KindChoice
		if optsJSON, err = json.Marshal(opts); err != nil {
			return nil, err
		}
		if p.Recommended > 0 {
			recKey = pgtype.Text{String: strconv.Itoa(p.Recommended), Valid: true}
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := dbq.New(tx)
	// The row lock serialises with closing the task, which withdraws its
	// open questions: a question cannot slip in after that sweep.
	t, err := q.GetTaskForUpdate(ctx, p.TaskID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && t.ProjectID != p.ProjectID) {
		return nil, tasks.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if tasks.IsClosed(tasks.State(t.State)) {
		return nil, errInvalid("the task is closed; questions go on open tasks")
	}
	if p.StepID != nil {
		_, err := q.GetTaskStep(ctx, dbq.GetTaskStepParams{ID: *p.StepID, TaskID: p.TaskID})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, tasks.ErrStepNotFound
		}
		if err != nil {
			return nil, err
		}
	}
	row, err := q.InsertTaskQuestion(ctx, dbq.InsertTaskQuestionParams{
		ProjectID:         p.ProjectID,
		TaskID:            p.TaskID,
		StepID:            p.StepID,
		SessionID:         p.SessionID,
		AskedByUserID:     p.AskedBy.UserID,
		AskedByTokenID:    p.AskedBy.TokenID,
		RecipientUserID:   p.AskedBy.UserID,
		Kind:              string(kind),
		BodyMd:            body,
		Options:           optsJSON,
		Multi:             p.Multi,
		RecommendedKey:    recKey,
		RecommendedReason: reason,
	})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return fromRow(row)
}

func timestampPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func fromRow(r dbq.TaskQuestion) (*Question, error) {
	q := &Question{
		ID:                r.ID,
		ProjectID:         r.ProjectID,
		TaskID:            r.TaskID,
		StepID:            r.StepID,
		SessionID:         r.SessionID,
		AskedByUserID:     r.AskedByUserID,
		RecipientUserID:   r.RecipientUserID,
		AnsweredByUserID:  r.AnsweredByUserID,
		Kind:              Kind(r.Kind),
		Body:              r.BodyMd,
		Multi:             r.Multi,
		RecommendedKey:    r.RecommendedKey.String,
		RecommendedReason: r.RecommendedReason,
		State:             State(r.State),
		AnsweredAt:        timestampPtr(r.AnsweredAt),
		DeliveredAt:       timestampPtr(r.DeliveredAt),
		WithdrawnAt:       timestampPtr(r.WithdrawnAt),
		WithdrawnReason:   r.WithdrawnReason,
		CreatedAt:         r.CreatedAt.Time,
		UpdatedAt:         r.UpdatedAt.Time,
	}
	if len(r.Options) > 0 {
		if err := json.Unmarshal(r.Options, &q.Options); err != nil {
			return nil, fmt.Errorf("question %s: options: %w", r.ID, err)
		}
	}
	if q.State == StateAnswered {
		q.Answer = &Reply{Keys: r.AnswerKeys, Text: r.AnswerText, Note: r.AnswerNote}
	}
	return q, nil
}

func fromRows(rows []dbq.TaskQuestion) ([]Question, error) {
	out := make([]Question, 0, len(rows))
	for _, r := range rows {
		q, err := fromRow(r)
		if err != nil {
			return nil, err
		}
		out = append(out, *q)
	}
	return out, nil
}
