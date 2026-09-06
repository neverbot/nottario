package tasks_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neverbot/nottario/internal/identity"
	"github.com/neverbot/nottario/internal/tasks"
	"github.com/neverbot/nottario/internal/testutil"
)

// priorityFixture spins up a project and returns everything the
// priority tests need to create a task in it.
func priorityFixture(t *testing.T) (context.Context, *pgxpool.Pool, identity.Project, tasks.Authorship) {
	t.Helper()
	pool := testutil.NewPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	u, _, err := identity.UpsertFromGithub(ctx, pool, 9101, "prio", "Prio", "")
	if err != nil {
		t.Fatalf("UpsertFromGithub: %v", err)
	}
	p, err := identity.CreateProject(ctx, pool, "PrioProj", "", "", "", u.ID)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return ctx, pool, *p, tasks.Authorship{UserID: &u.ID}
}

func TestCreateRejectsPriorityAboveRange(t *testing.T) {
	ctx, pool, p, by := priorityFixture(t)
	over := identity.MaxPriorityValue + 1
	_, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "too big", Priority: &over,
	}, by)
	if err == nil {
		t.Fatal("Create accepted a priority above the range")
	}
	if !strings.Contains(err.Error(), "priority must be between") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateRejectsNegativePriority(t *testing.T) {
	ctx, pool, p, by := priorityFixture(t)
	neg := -1
	if _, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "negative", Priority: &neg,
	}, by); err == nil {
		t.Fatal("Create accepted a negative priority")
	}
}

func TestCreateAcceptsOffBucketPriority(t *testing.T) {
	// The point of the change is to bound the scale, not to force
	// tasks onto buckets: interposing between two buckets stays legal.
	ctx, pool, p, by := priorityFixture(t)
	between := 70 // the seeded catalogue has 60 and 90, nothing at 70
	tk, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "between buckets", Priority: &between,
	}, by)
	if err != nil {
		t.Fatalf("Create rejected a legitimate off-bucket priority: %v", err)
	}
	if tk.Priority != between {
		t.Errorf("priority = %d, want %d", tk.Priority, between)
	}
}

func TestCreateAcceptsRangeBoundaries(t *testing.T) {
	ctx, pool, p, by := priorityFixture(t)
	for _, v := range []int{identity.MinPriorityValue, identity.MaxPriorityValue} {
		value := v
		if _, err := tasks.Create(ctx, pool, tasks.CreateParams{
			ProjectID: p.ID, Type: tasks.TypeTask, Title: "boundary", Priority: &value,
		}, by); err != nil {
			t.Errorf("Create rejected boundary %d: %v", value, err)
		}
	}
}

func TestUpdateRejectsPriorityOutsideRange(t *testing.T) {
	ctx, pool, p, by := priorityFixture(t)
	tk, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "updatable",
	}, by)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	over := identity.MaxPriorityValue + 1
	if _, err := tasks.Update(ctx, pool, tk.ID, tasks.UpdateParams{Priority: &over}); err == nil {
		t.Fatal("Update accepted a priority above the range")
	}
	// The rejected write must not have landed.
	got, err := tasks.Get(ctx, pool, tk.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Priority != tk.Priority {
		t.Errorf("priority changed to %d despite the rejection", got.Priority)
	}
}

func TestUpdateWithoutPriorityIsUnaffected(t *testing.T) {
	// A nil Priority means "leave unchanged" and must not trip the
	// bound check.
	ctx, pool, p, by := priorityFixture(t)
	tk, err := tasks.Create(ctx, pool, tasks.CreateParams{
		ProjectID: p.ID, Type: tasks.TypeTask, Title: "old title",
	}, by)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	title := "new title"
	got, err := tasks.Update(ctx, pool, tk.ID, tasks.UpdateParams{Title: &title})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Title != title || got.Priority != tk.Priority {
		t.Errorf("title=%q priority=%d", got.Title, got.Priority)
	}
}

func TestPriorityBucketRangeMatchesTaskRange(t *testing.T) {
	// The two scales must agree, or a legal bucket value would be an
	// illegal task priority.
	ctx, pool, p, _ := priorityFixture(t)
	if _, err := identity.UpsertPriority(ctx, pool, p.ID, "ceiling", identity.MaxPriorityValue, 9); err != nil {
		t.Errorf("UpsertPriority rejected the shared maximum: %v", err)
	}
	if _, err := identity.UpsertPriority(ctx, pool, p.ID, "over", identity.MaxPriorityValue+1, 10); err == nil {
		t.Error("UpsertPriority accepted a value above the shared maximum")
	}
}
