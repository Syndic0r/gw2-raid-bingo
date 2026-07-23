-- Rework the game model: a game is identified by its SET OF POOLS, not a fixed
-- instance/wing. Former per-wing "instance" pools become ordinary, deletable pools;
-- games gain a human name and a canonical pool-set key (one open game per set).

-- 1) Collapse the pool kinds. The former per-wing 'instance' pools become ordinary
--    pools, indistinguishable from the old 'shared' pools. The kind column and its
--    CHECK are left in place (every pool is 'shared' now) deliberately: rebuilding
--    the pools table with foreign keys enabled would cascade-delete every entry, so
--    we only rewrite the data. Application code no longer reads kind.
UPDATE pools SET kind = 'shared' WHERE kind = 'instance';

-- 2) Abort any game still open: it was bound to a single wing under the old model.
--    Its cards stay as read-only history. finished_at is unix seconds (matches now()).
UPDATE games SET status = 'aborted', finished_at = CAST(strftime('%s', 'now') AS INTEGER)
 WHERE status = 'open';

-- 3) Replace the game identity. Drop the one-open-per-(guild,instance) index, add the
--    new columns, backfill a display name from the old instance, then drop instance.
DROP INDEX idx_games_one_open;
ALTER TABLE games ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE games ADD COLUMN pool_set_key TEXT NOT NULL DEFAULT '';
UPDATE games SET name = CASE instance
    WHEN 'w1' THEN 'Wing 1' WHEN 'w2' THEN 'Wing 2' WHEN 'w3' THEN 'Wing 3'
    WHEN 'w4' THEN 'Wing 4' WHEN 'w5' THEN 'Wing 5' WHEN 'w6' THEN 'Wing 6'
    WHEN 'w7' THEN 'Wing 7' WHEN 'w8' THEN 'Wing 8'
    WHEN 'htcm' THEN 'Harvest Temple CM'
    ELSE instance END;
ALTER TABLE games DROP COLUMN instance;

-- One open game per distinct pool-set per guild. All migrated games are non-open,
-- so their empty pool_set_key never collides.
CREATE UNIQUE INDEX idx_games_one_open ON games (guild_id, pool_set_key) WHERE status = 'open';

-- 4) scheduled_games: a schedule now persists a pool-set + name instead of an instance.
--    (No incoming foreign keys, and the column is unindexed, so ALTER is safe.)
ALTER TABLE scheduled_games DROP COLUMN instance;
ALTER TABLE scheduled_games ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE scheduled_games ADD COLUMN pool_ids TEXT NOT NULL DEFAULT '[]';

-- 5) tracked_messages: the maintained status message is now per game, not per instance.
--    Its primary key changes, so rebuild it (rows are just stale Discord message
--    pointers; dropping them only means the bot posts fresh status messages).
DROP TABLE tracked_messages;
CREATE TABLE tracked_messages (
    guild_id   TEXT    NOT NULL,
    game_id    INTEGER NOT NULL,
    kind       TEXT    NOT NULL,
    channel_id TEXT    NOT NULL,
    message_id TEXT    NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (guild_id, game_id, kind)
) STRICT;
