package store

import (
	"context"
	"database/sql"
	"errors"
)

// Message kinds.
const MsgStatus = "status"

// TrackedMessage is a bot-maintained message we edit in place, keyed to the game
// it reports on.
type TrackedMessage struct {
	GuildID   string
	GameID    int64
	Kind      string
	ChannelID string
	MessageID string
	UpdatedAt int64
}

// UpsertTrackedMessage records (or replaces) the message id for a (guild, game, kind).
func (s *Store) UpsertTrackedMessage(ctx context.Context, guildID string, gameID int64, kind, channelID, messageID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tracked_messages (guild_id, game_id, kind, channel_id, message_id, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (guild_id, game_id, kind)
		 DO UPDATE SET channel_id = excluded.channel_id, message_id = excluded.message_id, updated_at = excluded.updated_at`,
		guildID, gameID, kind, channelID, messageID, now())
	return err
}

// GetTrackedMessage returns the tracked message for a (guild, game, kind), or ErrNotFound.
func (s *Store) GetTrackedMessage(ctx context.Context, guildID string, gameID int64, kind string) (TrackedMessage, error) {
	var m TrackedMessage
	err := s.db.QueryRowContext(ctx,
		`SELECT guild_id, game_id, kind, channel_id, message_id, updated_at
		 FROM tracked_messages WHERE guild_id = ? AND game_id = ? AND kind = ?`,
		guildID, gameID, kind).
		Scan(&m.GuildID, &m.GameID, &m.Kind, &m.ChannelID, &m.MessageID, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TrackedMessage{}, ErrNotFound
	}
	if err != nil {
		return TrackedMessage{}, err
	}
	return m, nil
}
