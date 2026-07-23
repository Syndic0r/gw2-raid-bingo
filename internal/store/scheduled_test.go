package store

import (
	"context"
	"testing"
)

// A transiently-failed schedule must be recoverable: RescheduleForRetry returns the
// claimed (fired) row to the pending queue with a backoff, so a later tick picks it
// up again instead of it being silently marked skipped and lost.
func TestRescheduleForRetryReturnsToPending(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	poolA, _ := seedGuild(t, s)

	sched, err := s.CreateScheduledGame(ctx, guild, "night", []int64{poolA}, 1000, false, "u")
	if err != nil {
		t.Fatal(err)
	}

	// claim it: pending -> fired
	due, err := s.ClaimDueScheduled(ctx, 1000, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 {
		t.Fatalf("want 1 claimed, got %d", len(due))
	}
	// a fired row is not re-claimed
	if again, _ := s.ClaimDueScheduled(ctx, 1000, 100); len(again) != 0 {
		t.Fatalf("fired row should not be re-claimed, got %d", len(again))
	}

	// transient failure -> reschedule for retry with a backoff to 1060
	if err := s.RescheduleForRetry(ctx, sched.ID, 1060); err != nil {
		t.Fatal(err)
	}
	// not due yet before the backoff
	if early, _ := s.ClaimDueScheduled(ctx, 1059, 100); len(early) != 0 {
		t.Fatalf("should not be due before backoff, got %d", len(early))
	}
	// due again at/after the backoff: retried, not lost
	retry, err := s.ClaimDueScheduled(ctx, 1060, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(retry) != 1 || retry[0].ID != sched.ID {
		t.Fatalf("want the rescheduled row retried once, got %d", len(retry))
	}
}
