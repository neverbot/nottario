package tasks_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

func TestTasks_CreateSetStateAndDependencyCycle(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 9001, "tasker", "Tasker", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "TaskProj", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	roles, err := identity.ListRoles(ctx, pool, p.ID)
	if err != nil || len(roles) == 0 {
		t.Fatalf("ListRoles: %v len=%d", err, len(roles))
	}
	roleID := roles[0].ID

	by := tasks.Authorship{UserID: &u.ID}
	mk := func(title string) *tasks.Task {
		tk, err := tasks.Create(ctx, pool, tasks.CreateParams{
			ProjectID:    p.ID,
			Type:         tasks.TypeTask,
			Title:        title,
			TargetRoleID: &roleID,
		}, by)
		if err != nil {
			t.Fatalf("Create %q: %v", title, err)
		}
		return tk
	}

	a := mk("A")
	b := mk("B")
	c := mk("C")

	if a.State != tasks.StateTodo {
		t.Fatalf("expected todo on create, got %s", a.State)
	}

	if _, err := tasks.SetState(ctx, pool, a.ID, tasks.StateDoing); err != nil {
		t.Fatalf("SetState doing: %v", err)
	}
	got, err := tasks.Get(ctx, pool, a.ID)
	if err != nil || got.State != tasks.StateDoing || got.ActualStart == nil {
		t.Fatalf("after doing: state=%s actual_start=%v err=%v", got.State, got.ActualStart, err)
	}
	if _, err := tasks.SetState(ctx, pool, a.ID, tasks.StateDone); err != nil {
		t.Fatalf("SetState done: %v", err)
	}
	got, _ = tasks.Get(ctx, pool, a.ID)
	if got.State != tasks.StateDone || got.ActualEnd == nil {
		t.Fatalf("after done: state=%s actual_end=%v", got.State, got.ActualEnd)
	}

	// Linear chain: c depends on b, b depends on a — no cycle.
	if err := tasks.AddDependency(ctx, pool, b.ID, a.ID); err != nil {
		t.Fatalf("AddDependency b<-a: %v", err)
	}
	if err := tasks.AddDependency(ctx, pool, c.ID, b.ID); err != nil {
		t.Fatalf("AddDependency c<-b: %v", err)
	}

	// Closing the loop: a depends on c => would create a 3-node cycle.
	err = tasks.AddDependency(ctx, pool, a.ID, c.ID)
	if !errors.Is(err, tasks.ErrCycle) {
		t.Fatalf("expected ErrCycle, got %v", err)
	}

	if err := tasks.RemoveDependency(ctx, pool, c.ID, b.ID); err != nil {
		t.Fatalf("RemoveDependency: %v", err)
	}
}

// TestTasks_HTMLEntitiesDecodedAtBoundary mirrors the architecture
// equivalent: agents that hand us entity-encoded title or description
// payloads (a `Build &amp; deploy` style string the Lit renderer
// would otherwise surface as literal `&amp;`) get the entities
// decoded so the row holds plain UTF-8 from then on.
func TestTasks_HTMLEntitiesDecodedAtBoundary(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 9100, "decoder", "Decoder", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "DecodeProj", "", "", "", u.ID, nil)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	roleID := roles[0].ID

	by := tasks.Authorship{UserID: &u.ID}
	created, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID:     p.ID,
		Type:          tasks.TypeTask,
		Title:         "Build &amp; deploy: GHCR &lt;img&gt; tags",
		DescriptionMD: "Run &quot;docker push&quot; &amp; verify.",
		TargetRoleID:  &roleID,
	}, by)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Title != "Build & deploy: GHCR <img> tags" {
		t.Errorf("Create title not decoded: %q", created.Title)
	}
	if created.DescriptionMD != `Run "docker push" & verify.` {
		t.Errorf("Create description not decoded: %q", created.DescriptionMD)
	}

	newTitle := "Build &amp; ship v2"
	newDesc := "Switch &lt;flag&gt;."
	updated, err := tasks.Update(ctx, pool, created.ID, tasks.UpdateParams{
		Title:         &newTitle,
		DescriptionMD: &newDesc,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "Build & ship v2" {
		t.Errorf("Update title not decoded: %q", updated.Title)
	}
	if updated.DescriptionMD != "Switch <flag>." {
		t.Errorf("Update description not decoded: %q", updated.DescriptionMD)
	}
}
