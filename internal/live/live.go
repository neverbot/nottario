// Package live builds the Live view: who is working on what right
// now, what waits on the human, and what just finished.
package live

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/neverbot/nottario/internal/agents"
	"github.com/neverbot/nottario/internal/db/dbq"
	"github.com/neverbot/nottario/internal/questions"
)

// FinishedWindow is how far back the finished block looks.
const FinishedWindow = 24 * time.Hour

// Board is the Live read model for a set of projects.
type Board struct {
	// Waiting is rendered by the web layer, hence not in the JSON.
	Waiting  []questions.Question `json:"-"`
	Projects []Project            `json:"projects"`
	Working  []Item               `json:"working"`
	Finished []Finished           `json:"finished"`
}

// Project carries the per-project Live settings.
type Project struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	Slug             string    `json:"slug"`
	IdleAfterMinutes int       `json:"idle_after_minutes"`
}

// Item is a task in doing.
type Item struct {
	TaskID         uuid.UUID
	ProjectID      uuid.UUID
	Title          string
	Type           string
	RoleID         *uuid.UUID
	AssigneeUserID *uuid.UUID
	StartedAt      *time.Time
	LastActivityAt time.Time
	// Idle is set when the task is past its project's live_idle_minutes.
	Idle    bool
	Session *agents.Session
	// Steps are the steps in progress, newest first.
	Steps         []StepRef
	Progress      *Progress
	QuestionsOpen int
}

// Progress counts a task's done and total steps.
type Progress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// StepRef is a step a session is working on.
type StepRef struct {
	ID       uuid.UUID
	Position int
	Text     string
	Session  *agents.Session
}

// Finished is a task closed inside the window.
type Finished struct {
	TaskID         uuid.UUID
	ProjectID      uuid.UUID
	Title          string
	State          string
	AssigneeUserID *uuid.UUID
	EndedAt        time.Time
}

// Build reads the Live board for these projects at now, in a fixed
// number of queries however many rows there are. recipient scopes the
// Waiting block to the questions addressed to that user.
func Build(ctx context.Context, db dbq.DBTX, projectIDs []uuid.UUID, recipient uuid.UUID, now time.Time) (*Board, error) {
	b := &Board{
		Waiting: []questions.Question{}, Projects: []Project{},
		Working: []Item{}, Finished: []Finished{},
	}
	if len(projectIDs) == 0 {
		return b, nil
	}
	q := dbq.New(db)

	settings, err := q.ProjectsLiveSettings(ctx, projectIDs)
	if err != nil {
		return nil, err
	}
	idleAfter := make(map[uuid.UUID]time.Duration, len(settings))
	for _, s := range settings {
		b.Projects = append(b.Projects, Project{ID: s.ID, Name: s.Name, Slug: s.Slug, IdleAfterMinutes: int(s.LiveIdleMinutes)})
		idleAfter[s.ID] = time.Duration(s.LiveIdleMinutes) * time.Minute
	}

	working, err := q.LiveWorkingTasks(ctx, projectIDs)
	if err != nil {
		return nil, err
	}
	taskIDs := make([]uuid.UUID, 0, len(working))
	for _, w := range working {
		taskIDs = append(taskIDs, w.ID)
	}
	var steps []dbq.LiveInProgressStepsRow
	progress := map[uuid.UUID]Progress{}
	open := map[uuid.UUID]int{}
	if len(taskIDs) > 0 {
		if steps, err = q.LiveInProgressSteps(ctx, taskIDs); err != nil {
			return nil, err
		}
		prog, err := q.TaskStepProgress(ctx, taskIDs)
		if err != nil {
			return nil, err
		}
		for _, r := range prog {
			progress[r.TaskID] = Progress{Done: int(r.Done), Total: int(r.Total)}
		}
		counts, err := q.CountOpenTaskQuestions(ctx, taskIDs)
		if err != nil {
			return nil, err
		}
		for _, r := range counts {
			open[r.TaskID] = int(r.Open)
		}
	}

	var sessionIDs []uuid.UUID
	for _, w := range working {
		if w.ActiveSessionID != nil {
			sessionIDs = append(sessionIDs, *w.ActiveSessionID)
		}
	}
	for _, s := range steps {
		if s.StartedSessionID != nil {
			sessionIDs = append(sessionIDs, *s.StartedSessionID)
		}
	}
	sessions, err := agents.ByIDs(ctx, db, sessionIDs)
	if err != nil {
		return nil, err
	}
	sessionRef := func(id *uuid.UUID) *agents.Session {
		if id == nil {
			return nil
		}
		if s, ok := sessions[*id]; ok {
			return &s
		}
		return nil
	}

	stepsByTask := map[uuid.UUID][]StepRef{}
	for _, s := range steps {
		stepsByTask[s.TaskID] = append(stepsByTask[s.TaskID], StepRef{
			ID: s.ID, Position: int(s.Position), Text: s.Text, Session: sessionRef(s.StartedSessionID),
		})
	}
	for _, w := range working {
		it := Item{
			TaskID: w.ID, ProjectID: w.ProjectID, Title: w.Title, Type: w.Type,
			RoleID: w.TargetRoleID, AssigneeUserID: w.AssigneeUserID,
			LastActivityAt: w.LastActivityAt.Time,
			Idle:           now.Sub(w.LastActivityAt.Time) > idleAfter[w.ProjectID],
			Session:        sessionRef(w.ActiveSessionID),
			Steps:          stepsByTask[w.ID],
			QuestionsOpen:  open[w.ID],
		}
		if w.ActualStart.Valid {
			t := w.ActualStart.Time
			it.StartedAt = &t
		}
		if p, ok := progress[w.ID]; ok {
			it.Progress = &p
		}
		b.Working = append(b.Working, it)
	}

	finished, err := q.LiveFinishedTasks(ctx, dbq.LiveFinishedTasksParams{
		ProjectIds: projectIDs, Since: tstz(now.Add(-FinishedWindow)),
	})
	if err != nil {
		return nil, err
	}
	for _, f := range finished {
		b.Finished = append(b.Finished, Finished{
			TaskID: f.ID, ProjectID: f.ProjectID, Title: f.Title, State: f.State,
			AssigneeUserID: f.AssigneeUserID, EndedAt: f.ActualEnd.Time,
		})
	}

	openState := questions.StateOpen
	waiting, err := questions.List(ctx, db, questions.ListFilter{
		ProjectIDs: projectIDs, State: &openState, RecipientUserID: &recipient, Limit: waitingLimit,
	})
	if err != nil {
		return nil, err
	}
	b.Waiting = waiting
	return b, nil
}

// waitingLimit caps the Waiting block.
const waitingLimit = 100

func tstz(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
