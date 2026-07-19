package discord

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
)

// formatDuration renders a whole-second span as a spaced, readable string:
// "3m 1s", "1h 5m", "45s".
func formatDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	out := ""
	add := func(v int64, unit string) {
		if v == 0 {
			return
		}
		if out != "" {
			out += " "
		}
		out += fmt.Sprintf("%d%s", v, unit)
	}
	add(h, "h")
	add(m, "m")
	if s > 0 || out == "" {
		if out != "" {
			out += " "
		}
		out += fmt.Sprintf("%ds", s)
	}
	return out
}

// celebrate posts the win announcement to the guild's configured announcement
// channel: a message, the winning card image, and a few stats. Best-effort.
func (b *Bot) celebrate(ctx context.Context, guildID string, inst bingo.Instance, winningCardID int64) {
	settings, err := b.svc.Store().GetGuildSettings(ctx, guildID)
	if err != nil || settings.AnnounceChannelID == "" {
		return // nowhere configured to celebrate
	}
	game, err := b.svc.Store().LatestGame(ctx, guildID, inst)
	if err != nil {
		b.log.Printf("celebrate: load game: %v", err)
		return
	}
	card, err := b.svc.Store().GetCard(ctx, guildID, winningCardID)
	if err != nil {
		b.log.Printf("celebrate: load card: %v", err)
		return
	}

	view := cardView{
		title:    "BINGO! - " + inst.Label(),
		subtitle: "Winning card",
		card:     card,
		readOnly: true,
	}
	png, err := view.responseData()
	var files []*discordgo.File
	if err == nil {
		files = png.Files
	}

	duration := ""
	if game.FinishedAt > game.CreatedAt {
		duration = " in " + formatDuration(game.FinishedAt-game.CreatedAt)
	}
	embed := &discordgo.MessageEmbed{
		Title:       "🎉 Bingo!",
		Description: fmt.Sprintf("<@%s> won **%s** bingo%s!", game.WinnerUserID, inst.Label(), duration),
		Color:       0xf1c40f,
	}

	// Ping the participant role (if configured) so everyone knows the round
	// ended, and always ping the winner. AllowedMentions is set explicitly so
	// only these two are ever pinged - never @everyone.
	content := fmt.Sprintf("🎉 <@%s> called **BINGO** for %s!", game.WinnerUserID, inst.Label())
	allowed := &discordgo.MessageAllowedMentions{Users: []string{game.WinnerUserID}}
	if settings.ParticipantRoleID != "" {
		content = fmt.Sprintf("<@&%s> ", settings.ParticipantRoleID) + content
		allowed.Roles = []string{settings.ParticipantRoleID}
	}
	if _, err := b.session.ChannelMessageSendComplex(settings.AnnounceChannelID, &discordgo.MessageSend{
		Content:         content,
		Embeds:          []*discordgo.MessageEmbed{embed},
		Files:           files,
		AllowedMentions: allowed,
	}); err != nil {
		b.log.Printf("celebrate: send: %v", err)
	}
}
