package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/events"
	"github.com/Syndic0r/gw2-raid-bingo/internal/service"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// statusEmbed renders a game's live status as an embed.
func (b *Bot) statusEmbed(stats service.GameStats) *discordgo.MessageEmbed {
	e := &discordgo.MessageEmbed{
		Title: "Raid Bingo - " + stats.Game.Name,
		Color: 0x2ecc71,
	}
	var desc strings.Builder
	fmt.Fprintf(&desc, "**Players:** %d\n", stats.PlayerCount)

	switch stats.Game.Status {
	case store.StatusFinished:
		fmt.Fprintf(&desc, "**Winner:** <@%s>\n", stats.Game.WinnerUserID)
	case store.StatusAborted:
		desc.WriteString("**Status:** aborted\n")
	}

	if len(stats.Leaders) > 0 && stats.Game.Status == store.StatusOpen {
		desc.WriteString("\n**Closest to bingo:**\n")
		for n, p := range stats.Leaders {
			if n == 5 {
				break
			}
			fmt.Fprintf(&desc, "%d. <@%s> - %d/5 on best line (%d marked)\n", n+1, p.UserID, p.BestLine, p.Marked)
		}
	}
	if b.cfg.BaseURL != "" {
		fmt.Fprintf(&desc, "\n[Open the interactive board](%s)", b.cfg.BaseURL)
	}
	e.Description = desc.String()
	return e
}

// announceGameOpen posts a heads-up to the guild's announcement channel that a
// game just started, pinging the participant role if one is configured so
// players are notified. Used for both manually opened and scheduled games.
func (b *Bot) announceGameOpen(ctx context.Context, guildID string, game store.Game) {
	settings, err := b.svc.Store().GetGuildSettings(ctx, guildID)
	if err != nil || settings.AnnounceChannelID == "" {
		return
	}
	content := fmt.Sprintf("🎲 A new bingo game **%s** just started! Run `/bingo card` and pick it to join.", game.Name)
	allowed := &discordgo.MessageAllowedMentions{}
	if settings.ParticipantRoleID != "" {
		content = fmt.Sprintf("<@&%s> ", settings.ParticipantRoleID) + content
		allowed.Roles = []string{settings.ParticipantRoleID}
	}
	if b.cfg.BaseURL != "" {
		content += "\n" + b.cfg.BaseURL
	}
	if _, err := b.session.ChannelMessageSendComplex(settings.AnnounceChannelID, &discordgo.MessageSend{
		Content:         content,
		AllowedMentions: allowed,
	}); err != nil {
		b.log.Printf("announce game open: %v", err)
	}
}

// announceGameAborted tells the channel a game was ended without a winner, so
// players know the round is over and their cards are now read-only.
func (b *Bot) announceGameAborted(ctx context.Context, guildID string, game store.Game) {
	settings, err := b.svc.Store().GetGuildSettings(ctx, guildID)
	if err != nil || settings.AnnounceChannelID == "" {
		return
	}
	content := fmt.Sprintf("🛑 The bingo game **%s** was aborted. No winner this round; cards are now read-only.", game.Name)
	allowed := &discordgo.MessageAllowedMentions{}
	if settings.ParticipantRoleID != "" {
		content = fmt.Sprintf("<@&%s> ", settings.ParticipantRoleID) + content
		allowed.Roles = []string{settings.ParticipantRoleID}
	}
	if _, err := b.session.ChannelMessageSendComplex(settings.AnnounceChannelID, &discordgo.MessageSend{
		Content:         content,
		AllowedMentions: allowed,
	}); err != nil {
		b.log.Printf("announce game aborted: %v", err)
	}
}

// statusComponents is the "Deal me in" button attached to a game's status message.
func statusComponents(gameID int64) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Deal me in / My card", Style: discordgo.PrimaryButton, CustomID: fmt.Sprintf("deal:%d", gameID)},
		}},
	}
}

// refreshStatusMessage re-renders the tracked status message for a game, if one
// exists. Best-effort: failures are logged, not surfaced.
func (b *Bot) refreshStatusMessage(ctx context.Context, guildID string, gameID int64) {
	tracked, err := b.svc.Store().GetTrackedMessage(ctx, guildID, gameID, store.MsgStatus)
	if err != nil {
		return // no status message posted for this game
	}
	stats, err := b.svc.GameStatsForGame(ctx, guildID, gameID)
	if err != nil {
		return // leave the message as-is on error
	}
	components := statusComponents(gameID)
	if stats.Game.Status != store.StatusOpen {
		components = nil // no joining once the game is closed
	}
	_, err = b.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    tracked.ChannelID,
		ID:         tracked.MessageID,
		Embeds:     &[]*discordgo.MessageEmbed{b.statusEmbed(stats)},
		Components: &components,
	})
	if err != nil {
		b.log.Printf("edit status message: %v", err)
	}
}

// startEventBridge subscribes to the hub and keeps status messages current and
// fires the celebration when a game is won. It runs until ctx is cancelled.
func (b *Bot) startEventBridge(ctx context.Context) {
	sub := b.hub.SubscribeAll()
	go func() {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-sub.C:
				if !ok {
					return
				}
				func() {
					defer b.recoverGuard("event bridge")
					b.onEvent(ctx, e)
				}()
			}
		}
	}()
}

func (b *Bot) onEvent(ctx context.Context, e events.Event) {
	if e.GameID == 0 {
		return
	}
	b.refreshStatusMessage(ctx, e.GuildID, e.GameID)
	// Announce from here so a new game started ANYWHERE - a Discord command, the
	// website, or the scheduler - posts to the configured channel exactly once.
	switch e.Kind {
	case events.GameOpened, events.GameAborted:
		game, err := b.svc.Store().GetGame(ctx, e.GuildID, e.GameID)
		if err != nil {
			return
		}
		if e.Kind == events.GameOpened {
			b.announceGameOpen(ctx, e.GuildID, game)
		} else {
			b.announceGameAborted(ctx, e.GuildID, game)
		}
	case events.GameFinished:
		b.celebrate(ctx, e.GuildID, e.GameID, e.CardID)
	}
}
