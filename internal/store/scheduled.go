package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

// Scheduled game statuses.
const (
	SchedPending   = "pending"
	SchedFired     = "fired"
	SchedSkipped   = "skipped"
	SchedCancelled = "cancelled"
)

// MaxPendingSchedulesPerGuild bounds how many upcoming schedules a guild can
// stack up.
const MaxPendingSchedulesPerGuild = 50

// ScheduledGame is a future game creation: it persists the pool-set (and optional
// name) to open when the time comes, mirroring NewGame's inputs.
type ScheduledGame struct {
	ID          int64
	GuildID     string
	Name        string
	PoolIDs     []int64
	FireAt      int64
	ReplaceOpen bool
	CreatedBy   string
	Status      string
	CreatedAt   int64
	FiredAt     int64
}

// CreateScheduledGame records a future game, enforcing the per-guild cap. The pool
// ids are stored as-is and validated against the guild when the schedule fires
// (pools may be edited between scheduling and firing).
func (s *Store) CreateScheduledGame(ctx context.Context, guildID, name string, poolIDs []int64, fireAt int64, replace bool, createdBy string) (ScheduledGame, error) {
	cleanName, err := cleanGameName(name)
	if err != nil {
		return ScheduledGame{}, err
	}
	if len(poolIDs) == 0 {
		return ScheduledGame{}, ErrNoPoolsSelected
	}
	poolJSON, err := json.Marshal(poolIDs)
	if err != nil {
		return ScheduledGame{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ScheduledGame{}, err
	}
	defer tx.Rollback()

	var pending int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scheduled_games WHERE guild_id = ? AND status = ?`,
		guildID, SchedPending).Scan(&pending); err != nil {
		return ScheduledGame{}, err
	}
	if pending >= MaxPendingSchedulesPerGuild {
		return ScheduledGame{}, validationErr("this server already has the maximum of %d scheduled games", MaxPendingSchedulesPerGuild)
	}

	ts := now()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO scheduled_games (guild_id, name, pool_ids, fire_at, replace_open, created_by, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		guildID, cleanName, string(poolJSON), fireAt, boolInt(replace), createdBy, SchedPending, ts)
	if err != nil {
		return ScheduledGame{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ScheduledGame{}, err
	}
	if err := tx.Commit(); err != nil {
		return ScheduledGame{}, err
	}
	return ScheduledGame{
		ID: id, GuildID: guildID, Name: cleanName, PoolIDs: poolIDs, FireAt: fireAt,
		ReplaceOpen: replace, CreatedBy: createdBy, Status: SchedPending, CreatedAt: ts,
	}, nil
}

const scheduledColumns = `id, guild_id, name, pool_ids, fire_at, replace_open, created_by, status, created_at, COALESCE(fired_at, 0)`

// ListScheduledGames returns a guild's pending schedules, soonest first.
func (s *Store) ListScheduledGames(ctx context.Context, guildID string) ([]ScheduledGame, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+scheduledColumns+` FROM scheduled_games WHERE guild_id = ? AND status = ? ORDER BY fire_at`,
		guildID, SchedPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduled(rows)
}

// CancelScheduledGame cancels a pending schedule owned by the guild.
func (s *Store) CancelScheduledGame(ctx context.Context, guildID string, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE scheduled_games SET status = ? WHERE id = ? AND guild_id = ? AND status = ?`,
		SchedCancelled, id, guildID, SchedPending)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ClaimDueScheduled atomically claims up to limit pending schedules whose time
// has come (fire_at <= nowUnix), flipping them to 'fired' so a second scheduler
// pass cannot double-fire them. It returns the claimed rows for processing.
func (s *Store) ClaimDueScheduled(ctx context.Context, nowUnix int64, limit int) ([]ScheduledGame, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx,
		`SELECT `+scheduledColumns+` FROM scheduled_games WHERE status = ? AND fire_at <= ? ORDER BY fire_at LIMIT ?`,
		SchedPending, nowUnix, limit)
	if err != nil {
		return nil, err
	}
	due, err := scanScheduled(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range due {
		if _, err := tx.ExecContext(ctx,
			`UPDATE scheduled_games SET status = ?, fired_at = ? WHERE id = ? AND status = ?`,
			SchedFired, nowUnix, due[i].ID, SchedPending); err != nil {
			return nil, err
		}
		due[i].Status = SchedFired
		due[i].FiredAt = nowUnix
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return due, nil
}

// MarkScheduledSkipped records that a claimed schedule could not open a game for
// a PERMANENT reason (e.g. a game was already open and replace was not requested,
// or the pools were emptied so a card can no longer be filled). The schedule is
// done and will not be retried.
func (s *Store) MarkScheduledSkipped(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE scheduled_games SET status = ? WHERE id = ?`, SchedSkipped, id)
	return err
}

// RescheduleForRetry returns a claimed (fired) schedule to the pending queue so a
// later tick retries it, moving its fire_at to nextFireAt as a small backoff. Used
// when opening the game failed for a TRANSIENT reason (e.g. a DB hiccup), so the
// schedule is not silently dropped. Only acts on a still-fired row, so a row that
// was concurrently cancelled or opened is left alone.
func (s *Store) RescheduleForRetry(ctx context.Context, id, nextFireAt int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE scheduled_games SET status = ?, fire_at = ?, fired_at = NULL WHERE id = ? AND status = ?`,
		SchedPending, nextFireAt, id, SchedFired)
	return err
}

func scanScheduled(rows *sql.Rows) ([]ScheduledGame, error) {
	var out []ScheduledGame
	for rows.Next() {
		var (
			sg       ScheduledGame
			poolJSON string
			repl     int
		)
		if err := rows.Scan(&sg.ID, &sg.GuildID, &sg.Name, &poolJSON, &sg.FireAt, &repl, &sg.CreatedBy, &sg.Status, &sg.CreatedAt, &sg.FiredAt); err != nil {
			return nil, err
		}
		sg.ReplaceOpen = repl != 0
		if poolJSON != "" {
			if err := json.Unmarshal([]byte(poolJSON), &sg.PoolIDs); err != nil {
				return nil, err
			}
		}
		out = append(out, sg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
