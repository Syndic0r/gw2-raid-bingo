-- Web login sessions. The cookie holds a random token; only its SHA-256 hash is
-- stored here, so a database read cannot reveal a usable session token.
CREATE TABLE sessions (
    token_hash TEXT    PRIMARY KEY,
    user_id    TEXT    NOT NULL,
    username   TEXT    NOT NULL,
    avatar     TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
) STRICT;

CREATE INDEX idx_sessions_expiry ON sessions (expires_at);
