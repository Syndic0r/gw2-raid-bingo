-- The participant role is pinged when a game is won, so everyone playing knows a
-- round ended. Optional; empty means no ping.
ALTER TABLE guild_settings ADD COLUMN participant_role_id TEXT NOT NULL DEFAULT '';
