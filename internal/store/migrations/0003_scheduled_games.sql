-- Games an admin scheduled to open automatically at a future time. A background
-- scheduler in the bot claims due rows (pending -> fired) and creates the games.
CREATE TABLE scheduled_games (
    id           INTEGER PRIMARY KEY,
    guild_id     TEXT    NOT NULL,
    instance     TEXT    NOT NULL,
    fire_at      INTEGER NOT NULL,
    replace_open INTEGER NOT NULL DEFAULT 0,
    created_by   TEXT    NOT NULL,
    status       TEXT    NOT NULL CHECK (status IN ('pending', 'fired', 'skipped', 'cancelled')),
    created_at   INTEGER NOT NULL,
    fired_at     INTEGER
) STRICT;

CREATE INDEX idx_scheduled_due ON scheduled_games (status, fire_at);
CREATE INDEX idx_scheduled_guild ON scheduled_games (guild_id, status);
