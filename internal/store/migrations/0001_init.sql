-- Core game schema. All game data is scoped by guild_id (Discord snowflake,
-- stored as TEXT). Tables are STRICT so column types are enforced.

-- One row per guild the bot is active in.
CREATE TABLE guild_settings (
    guild_id            TEXT PRIMARY KEY,
    announce_channel_id TEXT    NOT NULL DEFAULT '',
    is_seed_guild       INTEGER NOT NULL DEFAULT 0,
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL
) STRICT;

-- Roles an admin explicitly granted bingo-admin rights via /setup (many per guild).
CREATE TABLE admin_roles (
    guild_id TEXT NOT NULL,
    role_id  TEXT NOT NULL,
    PRIMARY KEY (guild_id, role_id)
) STRICT;

-- Pools of bingo squares. kind='instance' pools map 1:1 to the nine instances
-- (slug = instance key); kind='shared' pools are named, admin-created pools
-- mixed into every card.
CREATE TABLE pools (
    id         INTEGER PRIMARY KEY,
    guild_id   TEXT    NOT NULL,
    kind       TEXT    NOT NULL CHECK (kind IN ('instance', 'shared')),
    slug       TEXT    NOT NULL,
    name       TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (guild_id, kind, slug)
) STRICT;

-- Individual bingo squares. Soft-deleted via active=0 so historical cards, which
-- snapshot their text, are never disturbed.
CREATE TABLE entries (
    id         INTEGER PRIMARY KEY,
    guild_id   TEXT    NOT NULL,
    pool_id    INTEGER NOT NULL REFERENCES pools (id) ON DELETE CASCADE,
    text       TEXT    NOT NULL,
    active     INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE INDEX idx_entries_pool_active ON entries (pool_id, active);

-- A game is one bingo round for an instance. winner_card_id has no hard foreign
-- key (cards reference games, so a hard link back would be circular); it is set
-- atomically with status by the call-bingo transaction.
CREATE TABLE games (
    id             INTEGER PRIMARY KEY,
    guild_id       TEXT    NOT NULL,
    instance       TEXT    NOT NULL,
    status         TEXT    NOT NULL CHECK (status IN ('open', 'finished', 'aborted')),
    created_by     TEXT    NOT NULL,
    pool_ids       TEXT    NOT NULL DEFAULT '[]', -- JSON array of shared pool ids in play
    created_at     INTEGER NOT NULL,
    finished_at    INTEGER,
    winner_user_id TEXT,
    winner_card_id INTEGER
) STRICT;

-- At most one open game per (guild, instance).
CREATE UNIQUE INDEX idx_games_one_open ON games (guild_id, instance) WHERE status = 'open';
CREATE INDEX idx_games_guild ON games (guild_id, status);

-- One player's card within a game.
CREATE TABLE cards (
    id         INTEGER PRIMARY KEY,
    game_id    INTEGER NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    guild_id   TEXT    NOT NULL,
    user_id    TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (game_id, user_id)
) STRICT;

-- The 25 squares of a card. entry_id is null for the free centre; text is
-- snapshotted at deal time.
CREATE TABLE card_cells (
    card_id  INTEGER NOT NULL REFERENCES cards (id) ON DELETE CASCADE,
    idx      INTEGER NOT NULL CHECK (idx >= 0 AND idx < 25),
    entry_id INTEGER,
    text     TEXT    NOT NULL,
    free     INTEGER NOT NULL DEFAULT 0,
    marked   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (card_id, idx)
) STRICT;
