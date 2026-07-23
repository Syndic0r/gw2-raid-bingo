package service

import (
	"context"
	"errors"

	"github.com/Syndic0r/gw2-raid-bingo/internal/events"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

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
			if errors.Is(err, store.ErrGameOpen) {
				// A game with this pool-set was already open and this schedule did
				// not ask to replace it.
				_ = s.store.MarkScheduledSkipped(ctx, sched.ID)
				out = append(out, FiredSchedule{Schedule: sched, Skipped: true})
				continue
			}
			// The pools may have been deleted or emptied since scheduling (no pools
			// left, or too few squares to fill a card), or a transient error. Mark it
			// skipped rather than retry-storming, and report it.
			_ = s.store.MarkScheduledSkipped(ctx, sched.ID)
			out = append(out, FiredSchedule{Schedule: sched, Skipped: true})
			continue
		}
		s.hub.Publish(events.Event{
			Kind: events.GameOpened, GuildID: sched.GuildID, GameID: game.ID, UserID: sched.CreatedBy,
		})
		out = append(out, FiredSchedule{Schedule: sched, Game: game})
	}
	return out, nil
}
