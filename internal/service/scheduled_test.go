package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

func TestScheduleRequiresAdmin(t *testing.T) {
	svc, st := newSvc(t, "admin")
	sharedID := seed(t, st)
	ctx := context.Background()
	if _, err := svc.ScheduleGame(ctx, guild, "rando", "", []int64{sharedID}, 9_000_000_000, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin schedule: got %v, want ErrForbidden", err)
	}
	sched, err := svc.ScheduleGame(ctx, guild, "admin", "", []int64{sharedID}, 9_000_000_000, false)
	if err != nil {
		t.Fatal(err)
	}
	if sched.Status != store.SchedPending {
		t.Fatalf("status = %q, want pending", sched.Status)
	}
}

func TestRunDueSchedulesOpensGame(t *testing.T) {
	svc, st := newSvc(t, "admin")
	sharedID := seed(t, st)
	ctx := context.Background()

	// A second dealable pool so the two schedules use distinct pool-sets (one open
	// game per set), otherwise the second would be skipped as already-open.
	poolB, _ := st.CreatePool(ctx, guild, "poolb", "Pool B")
	for i := 0; i < 24; i++ {
		st.AddEntry(ctx, guild, poolB.ID, "b "+string(rune('a'+i)))
	}

	// Schedule two distinct sets for a past time so both are due.
	if _, err := svc.ScheduleGame(ctx, guild, "admin", "", []int64{sharedID}, 1000, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ScheduleGame(ctx, guild, "admin", "", []int64{poolB.ID}, 1000, false); err != nil {
		t.Fatal(err)
	}

	fired, err := svc.RunDueSchedules(ctx, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(fired) != 2 {
		t.Fatalf("fired %d, want 2", len(fired))
	}
	for _, f := range fired {
		if f.Skipped {
			t.Errorf("schedule #%d unexpectedly skipped", f.Schedule.ID)
		}
		if f.Game.Status != store.StatusOpen {
			t.Errorf("game for schedule #%d not open", f.Schedule.ID)
		}
	}

	// A second pass finds nothing (already claimed), so no double-fire.
	again, err := svc.RunDueSchedules(ctx, 3000)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("second pass fired %d, want 0", len(again))
	}
}

func TestRunDueSchedulesSkipsWhenOpen(t *testing.T) {
	svc, st := newSvc(t, "admin")
	sharedID := seed(t, st)
	ctx := context.Background()

	// Open a game manually, then a schedule for the same pool-set without replace skips.
	if _, err := svc.NewGame(ctx, guild, "admin", "", []int64{sharedID}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ScheduleGame(ctx, guild, "admin", "", []int64{sharedID}, 1000, false); err != nil {
		t.Fatal(err)
	}
	fired, err := svc.RunDueSchedules(ctx, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(fired) != 1 || !fired[0].Skipped {
		t.Fatalf("expected one skipped schedule, got %+v", fired)
	}
}

func TestCancelScheduled(t *testing.T) {
	svc, st := newSvc(t, "admin")
	sharedID := seed(t, st)
	ctx := context.Background()
	sched, err := svc.ScheduleGame(ctx, guild, "admin", "", []int64{sharedID}, 9_000_000_000, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CancelScheduled(ctx, guild, "admin", sched.ID); err != nil {
		t.Fatal(err)
	}
	fired, _ := svc.RunDueSchedules(ctx, 9_999_999_999)
	if len(fired) != 0 {
		t.Fatalf("cancelled schedule still fired: %+v", fired)
	}
	if err := svc.CancelScheduled(ctx, guild, "admin", sched.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("re-cancel: got %v, want ErrNotFound", err)
	}
}
