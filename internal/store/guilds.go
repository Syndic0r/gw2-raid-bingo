package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// now returns the current unix time. It is a field-free helper so tests reading
// timestamps stay simple; monotonicity is not required by any query.
func now() int64 { return time.Now().Unix() }

// EnsureGuild inserts the guild's settings row if it is missing and creates the
// nine fixed instance pools for it. It is idempotent and safe to call on every
// interaction. Shared pools are never created here - guilds add their own.
func (s *Store) EnsureGuild(ctx context.Context, guildID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ts := now()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO guild_settings (guild_id, created_at, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT (guild_id) DO NOTHING`,
		guildID, ts, ts); err != nil {
		return fmt.Errorf("ensure guild: %w", err)
	}
	for _, inst := range bingo.Instances() {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO pools (guild_id, kind, slug, name, created_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT (guild_id, kind, slug) DO NOTHING`,
			guildID, KindInstance, string(inst), inst.Label(), ts); err != nil {
			return fmt.Errorf("ensure instance pool %s: %w", inst, err)
		}
	}
	return tx.Commit()
}

// GetGuildSettings returns the guild's settings, or ErrNotFound.
func (s *Store) GetGuildSettings(ctx context.Context, guildID string) (GuildSettings, error) {
	var g GuildSettings
	err := s.db.QueryRowContext(ctx,
		`SELECT guild_id, announce_channel_id, participant_role_id, is_seed_guild, created_at, updated_at
		 FROM guild_settings WHERE guild_id = ?`, guildID).
		Scan(&g.GuildID, &g.AnnounceChannelID, &g.ParticipantRoleID, &g.IsSeedGuild, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GuildSettings{}, ErrNotFound
	}
	return g, err
}

// SetParticipantRole records the role pinged when a game is won ("" to clear).
func (s *Store) SetParticipantRole(ctx context.Context, guildID, roleID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE guild_settings SET participant_role_id = ?, updated_at = ? WHERE guild_id = ?`,
		roleID, now(), guildID)
	return err
}

// SetAnnounceChannel records the celebration/announcement channel for a guild.
func (s *Store) SetAnnounceChannel(ctx context.Context, guildID, channelID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE guild_settings SET announce_channel_id = ?, updated_at = ? WHERE guild_id = ?`,
		channelID, now(), guildID)
	return err
}

// MarkSeedGuild flags a guild as the seed guild (only the configured home guild).
func (s *Store) MarkSeedGuild(ctx context.Context, guildID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE guild_settings SET is_seed_guild = 1, updated_at = ? WHERE guild_id = ?`,
		now(), guildID)
	return err
}

// SetAdminRoles replaces the guild's configured bingo-admin roles with the given
// set (deduplicated). Passing an empty slice clears them.
func (s *Store) SetAdminRoles(ctx context.Context, guildID string, roleIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_roles WHERE guild_id = ?`, guildID); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(roleIDs))
	for _, r := range roleIDs {
		if r == "" {
			continue
		}
		if _, dup := seen[r]; dup {
			continue
		}
		seen[r] = struct{}{}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO admin_roles (guild_id, role_id) VALUES (?, ?)`, guildID, r); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetAdminRoles returns the guild's configured bingo-admin role IDs.
func (s *Store) GetAdminRoles(ctx context.Context, guildID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT role_id FROM admin_roles WHERE guild_id = ? ORDER BY role_id`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
