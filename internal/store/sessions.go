package store

import (
	"context"
	"database/sql"
	"errors"
)

// Session is a web login session, keyed by the SHA-256 hash of the cookie token.
type Session struct {
	UserID    string
	Username  string
	Avatar    string
	CreatedAt int64
	ExpiresAt int64
}

// CreateSession stores a session by its token hash.
func (s *Store) CreateSession(ctx context.Context, tokenHash, userID, username, avatar string, expiresAt int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, username, avatar, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tokenHash, userID, username, avatar, now(), expiresAt)
	return err
}

// GetSession returns a live (unexpired) session by token hash, or ErrNotFound.
func (s *Store) GetSession(ctx context.Context, tokenHash string) (Session, error) {
	var sess Session
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, username, avatar, created_at, expires_at
		 FROM sessions WHERE token_hash = ? AND expires_at > ?`, tokenHash, now()).
		Scan(&sess.UserID, &sess.Username, &sess.Avatar, &sess.CreatedAt, &sess.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return sess, err
}

// DeleteSession removes a session (logout).
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// PurgeExpiredSessions deletes sessions past their expiry.
func (s *Store) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
