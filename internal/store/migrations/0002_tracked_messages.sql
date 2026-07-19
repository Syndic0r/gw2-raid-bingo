-- Bot-maintained messages we edit in place, e.g. the public game status message
-- with the "Deal me in" button. Keyed by (guild, instance, kind); one row per
-- kind per instance.
CREATE TABLE tracked_messages (
    guild_id   TEXT    NOT NULL,
    instance   TEXT    NOT NULL,
    kind       TEXT    NOT NULL,
    channel_id TEXT    NOT NULL,
    message_id TEXT    NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (guild_id, instance, kind)
) STRICT;
