package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
)

// Message kinds.
const MsgStatus = "status"

// TrackedMessage is a bot-maintained message we edit in place.
type TrackedMessage struct {
	GuildID   string
	Instance  bingo.Instance
	Kind      string
	ChannelID string
	MessageID string
	UpdatedAt int64
}

// UpsertTrackedMessage records (or replaces) the message id for a (guild,
// instance, kind).
func (s *Store) UpsertTrackedMessage(ctx context.Context, guildID string, inst bingo.Instance, kind, channelID, messageID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tracked_messages (guild_id, instance, kind, channel_id, message_id, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (guild_id, instance, kind)
		 DO UPDATE SET channel_id = excluded.channel_id, message_id = excluded.message_id, updated_at = excluded.updated_at`,
		guildID, string(inst), kind, channelID, messageID, now())
	return err
}

// GetTrackedMessage returns the tracked message for a (guild, instance, kind),
// or ErrNotFound.
func (s *Store) GetTrackedMessage(ctx context.Context, guildID string, inst bingo.Instance, kind string) (TrackedMessage, error) {
	var m TrackedMessage
	var inststr string
	err := s.db.QueryRowContext(ctx,
		`SELECT guild_id, instance, kind, channel_id, message_id, updated_at
		 FROM tracked_messages WHERE guild_id = ? AND instance = ? AND kind = ?`,
		guildID, string(inst), kind).
		Scan(&m.GuildID, &inststr, &m.Kind, &m.ChannelID, &m.MessageID, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TrackedMessage{}, ErrNotFound
	}
	if err != nil {
		return TrackedMessage{}, err
	}
	m.Instance = bingo.Instance(inststr)
	return m, nil
}

// DeleteTrackedMessage removes a tracked message record.
func (s *Store) DeleteTrackedMessage(ctx context.Context, guildID string, inst bingo.Instance, kind string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM tracked_messages WHERE guild_id = ? AND instance = ? AND kind = ?`,
		guildID, string(inst), kind)
	return err
}
