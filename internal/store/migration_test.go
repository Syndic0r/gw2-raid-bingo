package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

// oldSchemaV5 is the subset of the pre-0006 schema that migration 0006 rewrites,
// recorded at schema version 5 so Open() applies only 0006 on top of it.
const oldSchemaV5 = `
CREATE TABLE pools (
    id INTEGER PRIMARY KEY, guild_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('instance','shared')),
    slug TEXT NOT NULL, name TEXT NOT NULL, created_at INTEGER NOT NULL,
    UNIQUE (guild_id, kind, slug)) STRICT;
CREATE TABLE entries (
    id INTEGER PRIMARY KEY, guild_id TEXT NOT NULL,
    pool_id INTEGER NOT NULL REFERENCES pools(id) ON DELETE CASCADE,
    text TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL) STRICT;
CREATE TABLE games (
    id INTEGER PRIMARY KEY, guild_id TEXT NOT NULL, instance TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('open','finished','aborted')),
    created_by TEXT NOT NULL, pool_ids TEXT NOT NULL DEFAULT '[]',
    created_at INTEGER NOT NULL, finished_at INTEGER,
    winner_user_id TEXT, winner_card_id INTEGER) STRICT;
CREATE UNIQUE INDEX idx_games_one_open ON games (guild_id, instance) WHERE status = 'open';
CREATE INDEX idx_games_guild ON games (guild_id, status);
CREATE TABLE cards (
    id INTEGER PRIMARY KEY, game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    guild_id TEXT NOT NULL, user_id TEXT NOT NULL, created_at INTEGER NOT NULL,
    UNIQUE (game_id, user_id)) STRICT;
CREATE TABLE scheduled_games (
    id INTEGER PRIMARY KEY, guild_id TEXT NOT NULL, instance TEXT NOT NULL,
    fire_at INTEGER NOT NULL, replace_open INTEGER NOT NULL DEFAULT 0,
    created_by TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','fired','skipped','cancelled')),
    created_at INTEGER NOT NULL, fired_at INTEGER) STRICT;
CREATE TABLE tracked_messages (
    guild_id TEXT NOT NULL, instance TEXT NOT NULL, kind TEXT NOT NULL,
    channel_id TEXT NOT NULL, message_id TEXT NOT NULL, updated_at INTEGER NOT NULL,
    PRIMARY KEY (guild_id, instance, kind)) STRICT;
CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY) STRICT;
INSERT INTO schema_migrations (version) VALUES (1),(2),(3),(4),(5);

INSERT INTO pools (id, guild_id, kind, slug, name, created_at) VALUES
    (1, 'g', 'instance', 'w1', 'Wing 1 - Spirit Vale', 100);
INSERT INTO entries (guild_id, pool_id, text, active, created_at, updated_at) VALUES
    ('g', 1, 'square one', 1, 100, 100), ('g', 1, 'square two', 1, 100, 100);
INSERT INTO games (id, guild_id, instance, status, created_by, pool_ids, created_at) VALUES
    (1, 'g', 'w1', 'open', 'host', '[]', 100),
    (2, 'g', 'w2', 'finished', 'host', '[]', 50);
`

func TestMigration0006_ConvertsOldData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")

	// Build a database at schema version 5 with old-shape data.
	raw, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, oldSchemaV5); err != nil {
		raw.Close()
		t.Fatalf("seed v5: %v", err)
	}
	raw.Close()

	// Opening runs migration 0006 on top.
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open+migrate: %v", err)
	}
	defer s.Close()

	// The former instance pool is now an ordinary pool with its entries intact.
	p, err := s.GetPool(ctx, "g", "w1")
	if err != nil {
		t.Fatalf("w1 pool missing after migration: %v", err)
	}
	entries, _ := s.ListEntries(ctx, "g", p.ID, true)
	if len(entries) != 2 {
		t.Fatalf("entries lost in migration: got %d, want 2", len(entries))
	}
	// ...and it is now deletable (was an instance pool before).
	if err := s.DeletePool(ctx, "g", p.ID); err != nil {
		t.Fatalf("former wing pool not deletable: %v", err)
	}

	// The open game was aborted; the finished game kept its status and got a name.
	g1, err := s.GetGame(ctx, "g", 1)
	if err != nil {
		t.Fatal(err)
	}
	if g1.Status != StatusAborted {
		t.Fatalf("open game status after migration = %q, want aborted", g1.Status)
	}
	g2, err := s.GetGame(ctx, "g", 2)
	if err != nil {
		t.Fatal(err)
	}
	if g2.Status != StatusFinished || g2.Name != "Wing 2" {
		t.Fatalf("finished game after migration = %+v (want finished, name Wing 2)", g2)
	}

	// No open games remain, so a fresh pool-set game can be opened.
	open, _ := s.ListOpenGames(ctx, "g")
	if len(open) != 0 {
		t.Fatalf("open games after migration = %d, want 0", len(open))
	}
	if !errors.Is(func() error { _, e := s.GetPool(ctx, "g", "nope"); return e }(), ErrNotFound) {
		t.Fatal("sanity: GetPool for missing slug should be ErrNotFound")
	}
}
