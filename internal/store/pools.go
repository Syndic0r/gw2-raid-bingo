package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
)

// CreateSharedPool creates a named shared pool for a guild. It enforces the
// per-guild shared-pool cap and rejects duplicate slugs.
func (s *Store) CreateSharedPool(ctx context.Context, guildID, slug, name string) (Pool, error) {
	cleanSlugVal, err := cleanSlug(slug)
	if err != nil {
		return Pool{}, err
	}
	cleanNameVal, err := cleanPoolName(name)
	if err != nil {
		return Pool{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Pool{}, err
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pools WHERE guild_id = ? AND kind = ?`,
		guildID, KindShared).Scan(&count); err != nil {
		return Pool{}, err
	}
	if count >= MaxSharedPools {
		return Pool{}, validationErr("this server already has the maximum of %d shared pools", MaxSharedPools)
	}

	ts := now()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO pools (guild_id, kind, slug, name, created_at) VALUES (?, ?, ?, ?, ?)`,
		guildID, KindShared, cleanSlugVal, cleanNameVal, ts)
	if err != nil {
		if isUniqueViolation(err) {
			return Pool{}, validationErr("a shared pool with slug %q already exists", cleanSlugVal)
		}
		return Pool{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Pool{}, err
	}
	if err := tx.Commit(); err != nil {
		return Pool{}, err
	}
	return Pool{ID: id, GuildID: guildID, Kind: KindShared, Slug: cleanSlugVal, Name: cleanNameVal, CreatedAt: ts}, nil
}

// DeleteSharedPool removes a shared pool and its entries (cascade). Instance
// pools cannot be deleted.
func (s *Store) DeleteSharedPool(ctx context.Context, guildID string, poolID int64) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM pools WHERE id = ? AND guild_id = ? AND kind = ?`,
		poolID, guildID, KindShared)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPools returns a guild's pools of the given kind (KindInstance or
// KindShared), ordered by slug.
func (s *Store) ListPools(ctx context.Context, guildID, kind string) ([]Pool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, guild_id, kind, slug, name, created_at
		 FROM pools WHERE guild_id = ? AND kind = ? ORDER BY slug`, guildID, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Pool
	for rows.Next() {
		var p Pool
		if err := rows.Scan(&p.ID, &p.GuildID, &p.Kind, &p.Slug, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPool returns one pool by (guild, kind, slug), or ErrNotFound.
func (s *Store) GetPool(ctx context.Context, guildID, kind, slug string) (Pool, error) {
	var p Pool
	err := s.db.QueryRowContext(ctx,
		`SELECT id, guild_id, kind, slug, name, created_at
		 FROM pools WHERE guild_id = ? AND kind = ? AND slug = ?`, guildID, kind, slug).
		Scan(&p.ID, &p.GuildID, &p.Kind, &p.Slug, &p.Name, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Pool{}, ErrNotFound
	}
	return p, err
}

// InstancePool returns the fixed instance pool for inst in a guild.
func (s *Store) InstancePool(ctx context.Context, guildID string, inst bingo.Instance) (Pool, error) {
	return s.GetPool(ctx, guildID, KindInstance, string(inst))
}

// AddEntry adds a bingo square to a pool, enforcing the per-pool cap.
func (s *Store) AddEntry(ctx context.Context, guildID string, poolID int64, text string) (Entry, error) {
	cleaned, err := cleanText(text)
	if err != nil {
		return Entry{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Entry{}, err
	}
	defer tx.Rollback()

	// Confirm the pool belongs to the guild (prevents cross-guild writes).
	var exists int
	if err := tx.QueryRowContext(ctx,
		`SELECT 1 FROM pools WHERE id = ? AND guild_id = ?`, poolID, guildID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, ErrNotFound
		}
		return Entry{}, err
	}
	var active int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM entries WHERE pool_id = ? AND active = 1`, poolID).Scan(&active); err != nil {
		return Entry{}, err
	}
	if active >= MaxEntriesPerPool {
		return Entry{}, validationErr("this pool already has the maximum of %d entries", MaxEntriesPerPool)
	}

	ts := now()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO entries (guild_id, pool_id, text, active, created_at, updated_at)
		 VALUES (?, ?, ?, 1, ?, ?)`, guildID, poolID, cleaned, ts, ts)
	if err != nil {
		return Entry{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, err
	}
	return Entry{ID: id, GuildID: guildID, PoolID: poolID, Text: cleaned, Active: true, CreatedAt: ts, UpdatedAt: ts}, nil
}

// EditEntry updates an entry's text. It scopes by guild to prevent cross-guild edits.
func (s *Store) EditEntry(ctx context.Context, guildID string, entryID int64, text string) error {
	cleaned, err := cleanText(text)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE entries SET text = ?, updated_at = ? WHERE id = ? AND guild_id = ?`,
		cleaned, now(), entryID, guildID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SoftDeleteEntry marks an entry inactive so it is excluded from new cards while
// existing cards, which snapshot their text, are unaffected.
func (s *Store) SoftDeleteEntry(ctx context.Context, guildID string, entryID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE entries SET active = 0, updated_at = ? WHERE id = ? AND guild_id = ? AND active = 1`,
		now(), entryID, guildID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearPoolEntries soft-deletes every active entry in a pool (used to empty a
// static/instance pool, which cannot itself be deleted). Soft-delete keeps
// historical cards, which snapshot their text, intact. Returns how many were
// cleared. The pool must belong to the guild.
func (s *Store) ClearPoolEntries(ctx context.Context, guildID string, poolID int64) (int64, error) {
	var owned int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM pools WHERE id = ? AND guild_id = ?`, poolID, guildID).Scan(&owned)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE entries SET active = 0, updated_at = ? WHERE guild_id = ? AND pool_id = ? AND active = 1`,
		now(), guildID, poolID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListEntries returns a pool's entries. When activeOnly is false, soft-deleted
// entries are included too (for admin listing).
func (s *Store) ListEntries(ctx context.Context, guildID string, poolID int64, activeOnly bool) ([]Entry, error) {
	q := `SELECT id, guild_id, pool_id, text, active, created_at, updated_at
	      FROM entries WHERE guild_id = ? AND pool_id = ?`
	if activeOnly {
		q += ` AND active = 1`
	}
	q += ` ORDER BY id`
	return s.queryEntries(ctx, q, guildID, poolID)
}

func (s *Store) queryEntries(ctx context.Context, query string, args ...any) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.GuildID, &e.PoolID, &e.Text, &e.Active, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// toBingoEntries converts store entries to the bingo package's Entry type.
func toBingoEntries(entries []Entry) []bingo.Entry {
	out := make([]bingo.Entry, len(entries))
	for i, e := range entries {
		out[i] = bingo.Entry{ID: e.ID, Text: e.Text}
	}
	return out
}
