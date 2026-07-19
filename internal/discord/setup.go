package discord

import (
	"context"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// handleSetup shows the configuration controls: a channel picker for the
// announcement channel and a role picker for the bingo-admin roles. Each select
// applies immediately, so the flow is stateless. Gated on Discord Administrator.
func (b *Bot) handleSetup(ctx context.Context, i *discordgo.InteractionCreate) {
	ok, err := b.svc.CanConfigure(ctx, i.GuildID, interactionUserID(i))
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	if !ok {
		b.replyEphemeral(i, "Only server administrators can run /setup.")
		return
	}

	minRoles := 0
	oneRole := 1
	b.respond(i, &discordgo.InteractionResponseData{
		Flags: discordgo.MessageFlagsEphemeral,
		Content: "**Bingo setup**\n" +
			"1. Pick the channel where win celebrations are posted (**required** before a game can start; games can then be run from any channel).\n" +
			"2. Pick which roles may run bingo-admin commands (server admins always can).\n" +
			"3. Pick the participant role pinged when a game is won (optional).\n" +
			"Each choice is saved immediately.",
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:     discordgo.ChannelSelectMenu,
					CustomID:     "setup_channel",
					Placeholder:  "Announcement channel",
					ChannelTypes: []discordgo.ChannelType{discordgo.ChannelTypeGuildText},
				},
			}},
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:    discordgo.RoleSelectMenu,
					CustomID:    "setup_roles",
					Placeholder: "Bingo-admin roles (optional)",
					MinValues:   &minRoles,
					MaxValues:   20,
				},
			}},
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:    discordgo.RoleSelectMenu,
					CustomID:    "setup_participant",
					Placeholder: "Participant role, pinged on a win (optional)",
					MinValues:   &minRoles,
					MaxValues:   oneRole,
				},
			}},
		},
	})
}

// handleSetupParticipant saves (or clears) the participant role pinged on a win.
func (b *Bot) handleSetupParticipant(ctx context.Context, i *discordgo.InteractionCreate) {
	if !b.ensureCanConfigure(ctx, i) {
		return
	}
	roles := i.MessageComponentData().Values
	roleID := ""
	if len(roles) > 0 {
		roleID = roles[0]
	}
	if err := b.svc.Store().SetParticipantRole(ctx, i.GuildID, roleID); err != nil {
		b.ackEphemeralEdit(i, b.describeError(err))
		return
	}
	if roleID == "" {
		b.ackEphemeralEdit(i, "Participant role cleared. Wins will not ping a role.")
		return
	}
	b.ackEphemeralEdit(i, "Participant role set to <@&"+roleID+">. It will be pinged when a game is won.")
}

// handleSetupChannel saves the chosen announcement channel.
func (b *Bot) handleSetupChannel(ctx context.Context, i *discordgo.InteractionCreate) {
	if !b.ensureCanConfigure(ctx, i) {
		return
	}
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		b.ackEphemeralEdit(i, "No channel selected.")
		return
	}
	if err := b.svc.Store().SetAnnounceChannel(ctx, i.GuildID, values[0]); err != nil {
		b.ackEphemeralEdit(i, b.describeError(err))
		return
	}
	msg := "Announcement channel set to <#" + values[0] + ">. You can still set the bingo-admin roles below."
	if missing := b.missingChannelPerms(values[0]); len(missing) > 0 {
		msg += "\n\n⚠️ I can't post there yet - grant me **" + strings.Join(missing, ", ") +
			"** on that channel, or win announcements won't appear."
	}
	b.ackEphemeralEdit(i, msg)
}

// missingChannelPerms returns the message-posting permissions the bot lacks in a
// channel (empty if it can post). It is best-effort: if permissions can't be
// resolved, it returns nil rather than a false warning.
func (b *Bot) missingChannelPerms(channelID string) []string {
	if b.session.State == nil || b.session.State.User == nil {
		return nil
	}
	perms, err := b.session.UserChannelPermissions(b.session.State.User.ID, channelID)
	if err != nil {
		return nil
	}
	need := []struct {
		bit  int64
		name string
	}{
		{discordgo.PermissionViewChannel, "View Channel"},
		{discordgo.PermissionSendMessages, "Send Messages"},
		{discordgo.PermissionEmbedLinks, "Embed Links"},
		{discordgo.PermissionAttachFiles, "Attach Files"},
	}
	var missing []string
	for _, p := range need {
		if perms&p.bit == 0 {
			missing = append(missing, p.name)
		}
	}
	return missing
}

// handleSetupRoles saves the chosen bingo-admin roles (replacing the set).
func (b *Bot) handleSetupRoles(ctx context.Context, i *discordgo.InteractionCreate) {
	if !b.ensureCanConfigure(ctx, i) {
		return
	}
	roles := i.MessageComponentData().Values
	if err := b.svc.Store().SetAdminRoles(ctx, i.GuildID, roles); err != nil {
		b.ackEphemeralEdit(i, b.describeError(err))
		return
	}
	b.resolver.invalidate(i.GuildID)
	if len(roles) == 0 {
		b.ackEphemeralEdit(i, "Bingo-admin roles cleared. Only server administrators can run admin commands now.")
		return
	}
	b.ackEphemeralEdit(i, "Bingo-admin roles updated.")
}

func (b *Bot) ensureCanConfigure(ctx context.Context, i *discordgo.InteractionCreate) bool {
	ok, err := b.svc.CanConfigure(ctx, i.GuildID, interactionUserID(i))
	if err != nil {
		b.ackEphemeralEdit(i, b.describeError(err))
		return false
	}
	if !ok {
		b.ackEphemeralEdit(i, "Only server administrators can change these settings.")
		return false
	}
	return true
}

// ackEphemeralEdit updates the ephemeral setup message with feedback while
// keeping its controls in place.
func (b *Bot) ackEphemeralEdit(i *discordgo.InteractionCreate, msg string) {
	err := b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    msg,
			Flags:      discordgo.MessageFlagsEphemeral,
			Components: i.Message.Components,
		},
	})
	if err != nil {
		b.log.Printf("setup ack: %v", err)
	}
}
