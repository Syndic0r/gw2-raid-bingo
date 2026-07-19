package discord

// These methods expose the bot's gateway view of which guilds it is in, so the
// web server can build the guild picker (the user's guilds intersected with
// these) without a separate API call. They satisfy web.BotPresence.

// InGuild reports whether the bot is a member of the guild.
func (b *Bot) InGuild(guildID string) bool {
	if b.session.State == nil {
		return false
	}
	_, err := b.session.State.Guild(guildID)
	return err == nil
}

// GuildIDs returns the ids of every guild the bot is in.
func (b *Bot) GuildIDs() []string {
	if b.session.State == nil {
		return nil
	}
	b.session.State.RLock()
	defer b.session.State.RUnlock()
	ids := make([]string, 0, len(b.session.State.Guilds))
	for _, g := range b.session.State.Guilds {
		ids = append(ids, g.ID)
	}
	return ids
}

// GuildName returns the cached name of a guild, or "" if unknown.
func (b *Bot) GuildName(guildID string) string {
	if b.session.State == nil {
		return ""
	}
	if g, err := b.session.State.Guild(guildID); err == nil {
		return g.Name
	}
	return ""
}

// GuildIcon returns the cached icon hash of a guild, or "" if unknown.
func (b *Bot) GuildIcon(guildID string) string {
	if b.session.State == nil {
		return ""
	}
	if g, err := b.session.State.Guild(guildID); err == nil {
		return g.Icon
	}
	return ""
}
