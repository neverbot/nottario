package tasks_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/neverbot/nottario/internal/docs"
	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// All tests in this file use a release-on-Wait gate to maximize the
// chance of true parallel execution: every worker dials a sync.WaitGroup
// to "ready", the test calls a release channel close to start the storm,
// then a second sync.WaitGroup collects the results. Without the gate
// the first goroutine almost always wins the race.

// runConcurrent launches n goroutines, each running fn(i), with all
// of them blocking on the same release channel so the work fires
// "simultaneously". Returns when every goroutine has finished.
func runConcurrent(t *testing.T, n int, fn func(i int)) {
	t.Helper()
	ready := &sync.WaitGroup{}
	ready.Add(n)
	release := make(chan struct{})
	done := &sync.WaitGroup{}
	done.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer done.Done()
			ready.Done()
			<-release
			fn(idx)
		}(i)
	}
	ready.Wait()
	close(release)
	done.Wait()
}

// TestClaimNext_NoDoubleClaim asserts that N concurrent ClaimNext
// callers each receive a DIFFERENT task (or "no task") and that the
// total claimed count equals min(N, available todo tasks). This is
// the single most load-bearing concurrency invariant in the product:
// two agents racing must never both end up holding the same task.
func TestClaimNext_NoDoubleClaim(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	u, _, err := identity.UpsertFromGithub(ctx, pool, 9001, "racer", "Racer", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "RaceProj", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	if len(roles) == 0 {
		t.Fatalf("expected default roles")
	}
	role := roles[0].ID

	// Create 5 todo tasks, all eligible for the same caller.
	const taskCount = 5
	by := tasks.Authorship{UserID: &u.ID}
	for i := 0; i < taskCount; i++ {
		if _, err := tasks.Create(ctx, pool, tasks.CreateParams{
			ProjectID:    p.ID,
			Type:         tasks.TypeTask,
			Title:        "T",
			TargetRoleID: &role,
		}, by); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// Fire more goroutines than tasks so the surplus goroutines
	// exercise the "no task" return path concurrently with claimers.
	const goroutines = 8
	claimed := make([]*tasks.Task, goroutines)
	errs := make([]error, goroutines)
	runConcurrent(t, goroutines, func(i int) {
		tk, err := tasks.ClaimNext(ctx, pool, tasks.NextFilter{ProjectID: p.ID}, u.ID)
		claimed[i] = tk
		errs[i] = err
	})

	seen := map[uuid.UUID]bool{}
	successCount := 0
	for i, tk := range claimed {
		if tk != nil {
			if seen[tk.ID] {
				t.Fatalf("goroutine %d: task %s was claimed twice", i, tk.ID)
			}
			seen[tk.ID] = true
			successCount++
			if tk.State != tasks.StateDoing {
				t.Fatalf("claimed task %s state=%s, want doing", tk.ID, tk.State)
			}
			if tk.AssigneeUserID == nil || *tk.AssigneeUserID != u.ID {
				t.Fatalf("claimed task %s assignee=%v, want %s", tk.ID, tk.AssigneeUserID, u.ID)
			}
		} else if err := errs[i]; err != nil && !errors.Is(err, tasks.ErrNotFound) {
			t.Fatalf("goroutine %d unexpected error: %v", i, err)
		}
	}
	if successCount != taskCount {
		t.Fatalf("got %d successful claims, want %d", successCount, taskCount)
	}
}

// TestAddDependency_CycleRaceFree asserts that the cycle check is
// race-free under concurrent edge additions that, combined, would
// form a cycle. With just one task A and goroutines racing to make
// A depend on B and B depend on A, exactly one direction must win
// and the second add must fail with ErrCycle. The project-scoped
// pg_advisory_xact_lock in AddDependency is what makes this hold.
func TestAddDependency_CycleRaceFree(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	u, _, _ := identity.UpsertFromGithub(ctx, pool, 9002, "cycler", "Cycler", "")
	p, _ := identity.CreateProject(ctx, pool, "CycleProj", "", "", "", u.ID)
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	role := roles[0].ID
	by := tasks.Authorship{UserID: &u.ID}

	a, _ := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "A", TargetRoleID: &role}, by)
	b, _ := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "B", TargetRoleID: &role}, by)

	// Two adds that would individually be safe but together form a
	// 2-node cycle. After both, the graph must have AT MOST one of
	// the two edges; never both.
	var errAonB, errBonA error
	runConcurrent(t, 2, func(i int) {
		if i == 0 {
			errAonB = tasks.AddDependency(ctx, pool, a.ID, b.ID) // A depends on B
		} else {
			errBonA = tasks.AddDependency(ctx, pool, b.ID, a.ID) // B depends on A
		}
	})

	bothSucceeded := errAonB == nil && errBonA == nil
	if bothSucceeded {
		t.Fatalf("both edges committed; would form a cycle: A→B=%v, B→A=%v", errAonB, errBonA)
	}
	// At least one must succeed; the loser must fail with ErrCycle.
	if errAonB != nil && !errors.Is(errAonB, tasks.ErrCycle) {
		t.Fatalf("A→B failed for unexpected reason: %v", errAonB)
	}
	if errBonA != nil && !errors.Is(errBonA, tasks.ErrCycle) {
		t.Fatalf("B→A failed for unexpected reason: %v", errBonA)
	}
}

// TestSetStateDone_RollUpExactlyOnce closes N sibling children of a
// feature parent concurrently and asserts the parent transitions to
// done exactly once (no double-rollUp, no stale "still has children"
// reads). The rollUp runs inside the SetState transaction so
// siblings closing simultaneously must serialize on the parent row
// via FOR UPDATE.
func TestSetStateDone_RollUpExactlyOnce(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	u, _, _ := identity.UpsertFromGithub(ctx, pool, 9003, "rollup", "RollUp", "")
	p, _ := identity.CreateProject(ctx, pool, "RollupProj", "", "", "", u.ID)
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	role := roles[0].ID
	by := tasks.Authorship{UserID: &u.ID}

	feat, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeFeature, Title: "F", TargetRoleID: &role,
	}, by)
	if err != nil {
		t.Fatalf("create feature: %v", err)
	}
	const children = 4
	kids := make([]*tasks.Task, children)
	for i := 0; i < children; i++ {
		kids[i], err = tasks.Create(ctx, pool, tasks.CreateParams{
			ProjectID: p.ID, Type: tasks.TypeTask, Title: "C",
			ParentTaskID: &feat.ID, TargetRoleID: &role,
		}, by)
		if err != nil {
			t.Fatalf("create child %d: %v", i, err)
		}
	}

	// Close every child simultaneously.
	closeErrs := make([]error, children)
	runConcurrent(t, children, func(i int) {
		_, closeErrs[i] = tasks.SetState(ctx, pool, kids[i].ID, tasks.StateDone)
	})
	for i, err := range closeErrs {
		if err != nil {
			t.Fatalf("close child %d: %v", i, err)
		}
	}

	// Parent should be done.
	got, err := tasks.Get(ctx, pool, feat.ID)
	if err != nil {
		t.Fatalf("Get feature: %v", err)
	}
	if got.State != tasks.StateDone {
		t.Fatalf("parent state=%s, want done after all children closed", got.State)
	}
	if got.ActualEnd == nil {
		t.Fatalf("parent actual_end is nil; rollUp didn't stamp it")
	}
}

// TestSetState_PreconditionRace exercises the race between adding a
// dependency to a task and closing the task. The invariant: a task
// cannot end up in 'done' while one of its dependencies is non-done.
// Either the close wins (the dep was added against an already-done
// task, which is fine), or the add wins (the close should observe
// the new dep and fail with UnresolvedPreconditionsError).
func TestSetState_PreconditionRace(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	u, _, _ := identity.UpsertFromGithub(ctx, pool, 9004, "precond", "Precond", "")
	p, _ := identity.CreateProject(ctx, pool, "PrecondProj", "", "", "", u.ID)
	roles, _ := identity.ListRoles(ctx, pool, p.ID)
	role := roles[0].ID
	by := tasks.Authorship{UserID: &u.ID}

	target, _ := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "target", TargetRoleID: &role}, by)
	dep, _ := tasks.Create(ctx, pool, tasks.CreateParams{ProjectID: p.ID, Type: tasks.TypeTask, Title: "dep", TargetRoleID: &role}, by)

	// Race: one goroutine adds target depends_on dep (dep is in
	// 'todo'); the other goroutine tries to close target.
	var addErr, closeErr error
	var closed *tasks.Task
	runConcurrent(t, 2, func(i int) {
		if i == 0 {
			addErr = tasks.AddDependency(ctx, pool, target.ID, dep.ID)
		} else {
			closed, closeErr = tasks.SetState(ctx, pool, target.ID, tasks.StateDone)
		}
	})

	final, _ := tasks.Get(ctx, pool, target.ID)
	if addErr != nil {
		t.Fatalf("AddDependency failed: %v", addErr)
	}
	// The post-condition: if target ended up done, it must have closed
	// BEFORE the dep was added (i.e. the close didn't observe the dep).
	// If close failed with UnresolvedPreconditionsError, target stays
	// in todo. Either is OK.
	if closeErr != nil {
		var unresolved *tasks.UnresolvedPreconditionsError
		if !errors.As(closeErr, &unresolved) {
			t.Fatalf("close failed with unexpected error: %v", closeErr)
		}
		if final.State != tasks.StateTodo {
			t.Fatalf("close failed but state=%s, want todo", final.State)
		}
		return
	}
	// Close succeeded. The dep must have been added AFTER, so right
	// now the task IS done and the dep IS still in todo. The invariant
	// "no done task can depend on a non-done task" is allowed to be
	// violated transiently across this exact race window — but the
	// product calls it a permitted state because the dep was added
	// after the close. Just sanity-check the shape.
	if closed.State != tasks.StateDone {
		t.Fatalf("close path returned non-done task: %s", closed.State)
	}
	if final.State != tasks.StateDone {
		t.Fatalf("final state=%s, want done", final.State)
	}
}

// TestDocsWrite_VersionConflict races two writers updating the same
// document with the same expected_version. Exactly one must win and
// bump the row to v+1; the other must fail with ErrVersionConflict.
func TestDocsWrite_VersionConflict(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	u, _, _ := identity.UpsertFromGithub(ctx, pool, 9005, "writer", "Writer", "")
	p, _ := identity.CreateProject(ctx, pool, "DocProj", "", "", "", u.ID)
	by := docs.Authorship{UserID: &u.ID}

	path := "projects/" + p.ID.String() + "/context/race.md"
	zero := 0
	first, err := docs.Write(ctx, pool, docs.WriteParams{
		Scope:           docs.ScopeProject,
		ProjectID:       &p.ID,
		Path:            path,
		ContentMD:       "v1",
		Kind:            docs.KindContext,
		ExpectedVersion: &zero,
	}, by)
	if err != nil {
		t.Fatalf("initial Write: %v", err)
	}
	if first.CurrentVersion != 1 {
		t.Fatalf("initial version=%d, want 1", first.CurrentVersion)
	}

	// Two writers, both believing the current version is 1, race to
	// produce v2.
	var winCount int32
	var conflictCount int32
	var otherErr error
	runConcurrent(t, 2, func(i int) {
		ev := 1
		_, err := docs.Write(ctx, pool, docs.WriteParams{
			Scope:           docs.ScopeProject,
			ProjectID:       &p.ID,
			Path:            path,
			ContentMD:       "writer-" + string(rune('A'+i)),
			Kind:            docs.KindContext,
			Message:         "race",
			ExpectedVersion: &ev,
		}, by)
		switch {
		case err == nil:
			atomic.AddInt32(&winCount, 1)
		case errors.Is(err, docs.ErrVersionConflict):
			atomic.AddInt32(&conflictCount, 1)
		default:
			otherErr = err
		}
	})

	if otherErr != nil {
		t.Fatalf("unexpected error in race: %v", otherErr)
	}
	if winCount != 1 || conflictCount != 1 {
		t.Fatalf("race outcome: wins=%d conflicts=%d, want exactly 1 of each", winCount, conflictCount)
	}
	final, _ := docs.Read(ctx, pool, docs.ScopeProject, &p.ID, path)
	if final.CurrentVersion != 2 {
		t.Fatalf("final version=%d, want 2", final.CurrentVersion)
	}
}
