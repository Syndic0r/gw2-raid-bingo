package service

import (
	"context"
	"errors"

	"github.com/Syndic0r/gw2-raid-bingo/internal/events"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// scheduleRetryBackoffSec is how far ahead a transiently-failed schedule's fire_at
// is pushed before it becomes due again, so a retry does not storm every tick.
const scheduleRetryBackoffSec int64 = 60

// isPermanentScheduleError reports whether a NewGame failure at fire time is a
// permanent condition (the schedule should be skipped) rather than a transient one
// (it should be retried). Permanent: the exact pool-set is already open without
// replace (ErrGameOpen), no pools remain (ErrNoPoolsSelected), or the pools no
// longer offer enough squares / other input validation (ErrValidation).
func isPermanentScheduleError(err error) bool {
	return errors.Is(err, store.ErrGameOpen) ||
		errors.Is(err, store.ErrNoPoolsSelected) ||
		errors.Is(err, store.ErrValidation)
}

// ScheduleGame records a future game drawing from the given pools (admin only).
// name is an optional label (empty -> derived at fire time). fireAtUnix is the
// resolved unix time; validation of "future" and horizon happens before this in
// the bot layer (via the when package).
func (s *Service) ScheduleGame(ctx context.Context, guildID, userID, name string, poolIDs []int64, fireAtUnix int64, replace bool) (store.ScheduledGame, error) {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return store.ScheduledGame{}, err
	}
	if err := s.requireAnnounceChannel(ctx, guildID); err != nil {
		return store.ScheduledGame{}, err
	}
	return s.store.CreateScheduledGame(ctx, guildID, name, poolIDs, fireAtUnix, replace, userID)
}

// ListScheduled returns a guild's pending schedules (admin only).
func (s *Service) ListScheduled(ctx context.Context, guildID, userID string) ([]store.ScheduledGame, error) {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return nil, err
	}
	return s.store.ListScheduledGames(ctx, guildID)
}

// CancelScheduled cancels a pending schedule (admin only).
func (s *Service) CancelScheduled(ctx context.Context, guildID, userID string, id int64) error {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return err
	}
	return s.store.CancelScheduledGame(ctx, guildID, id)
}

// FiredSchedule reports the outcome of one due schedule the scheduler processed.
type FiredSchedule struct {
	Schedule store.ScheduledGame
	Game     store.Game // zero if skipped
	Skipped  bool       // true when a game was already open and replace was not set
}

// RunDueSchedules claims and opens every schedule due at nowUnix and returns the
// outcomes so the caller can announce them. It bypasses the per-action admin
// check on purpose: the schedule was authorized by an admin when created, and
// this runs unattended in the background. Each opened game still publishes its
// GameOpened event through the hub.
func (s *Service) RunDueSchedules(ctx context.Context, nowUnix int64) ([]FiredSchedule, error) {
	due, err := s.store.ClaimDueScheduled(ctx, nowUnix, 100)
	if err != nil {
		return nil, err
	}
	out := make([]FiredSchedule, 0, len(due))
	for _, sched := range due {
		game, err := s.store.NewGame(ctx, sched.GuildID, sched.Name, sched.CreatedBy, sched.PoolIDs, sched.ReplaceOpen)
		if err != nil {
			if isPermanentScheduleError(err) {
				// A permanent reason: a game with this pool-set was already open and
				// this schedule did not ask to replace it (ErrGameOpen), or the pools
				// were deleted/emptied since scheduling so no card can be filled
				// (validation). The schedule is done; mark it skipped and report it.
				_ = s.store.MarkScheduledSkipped(ctx, sched.ID)
				out = append(out, FiredSchedule{Schedule: sched, Skipped: true})
				continue
			}
			// Transient (e.g. a DB hiccup or a cancelled context): do NOT drop the
			// schedule. Return it to the pending queue with a short backoff so a later
			// tick retries it, instead of silently marking it skipped forever.
			_ = s.store.RescheduleForRetry(ctx, sched.ID, nowUnix+scheduleRetryBackoffSec)
			continue
		}
		s.hub.Publish(events.Event{
			Kind: events.GameOpened, GuildID: sched.GuildID, GameID: game.ID, UserID: sched.CreatedBy,
		})
		out = append(out, FiredSchedule{Schedule: sched, Game: game})
	}
	return out, nil
}
