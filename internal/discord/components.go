package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
)

// routeComponent dispatches a button or select-menu interaction by the prefix of
// its custom id.
func (b *Bot) routeComponent(ctx context.Context, i *discordgo.InteractionCreate) {
	id := i.MessageComponentData().CustomID
	switch {
	case id == "setup_channel":
		b.handleSetupChannel(ctx, i)
	case id == "setup_roles":
		b.handleSetupRoles(ctx, i)
	case id == "setup_participant":
		b.handleSetupParticipant(ctx, i)
	case strings.HasPrefix(id, "tog:"):
		b.handleToggle(ctx, i, id)
	case strings.HasPrefix(id, "call:"):
		b.handleCall(ctx, i, id)
	case strings.HasPrefix(id, "deal:"):
		b.handleDealButton(ctx, i, id)
	case strings.HasPrefix(id, "newreplace:"):
		b.handleNewReplace(ctx, i, id)
	case strings.HasPrefix(id, "abort:"):
		b.handleAbortConfirm(ctx, i, id)
	case strings.HasPrefix(id, "inspect:"):
		b.handleInspectSelect(ctx, i, id)
	case strings.HasPrefix(id, "clearpool:"):
		b.handleClearPool(ctx, i, id)
	case strings.HasPrefix(id, "sched:"):
		b.handleScheduleSelect(ctx, i, id)
	default:
		b.replyEphemeral(i, "This control is no longer active.")
	}
}

func (b *Bot) handleToggle(ctx context.Context, i *discordgo.InteractionCreate, id string) {
	parts := parseIDArgs(id) // tog:<cardID>:<idx>
	if len(parts) != 3 {
		b.replyEphemeral(i, "Invalid button.")
		return
	}
	cardID, ok1 := atoi64(parts[1])
	idx, ok2 := atoi64(parts[2])
	if !ok1 || !ok2 {
		b.replyEphemeral(i, "Invalid button.")
		return
	}
	card, hasBingo, err := b.svc.ToggleCell(ctx, i.GuildID, interactionUserID(i), cardID, int(idx))
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	sub := fmt.Sprintf("Card #%d - mark your squares", card.ID)
	if hasBingo {
		sub = "🎉 You have a line! Press the green BINGO button to win."
	}
	b.updateCardView(i, cardView{title: "Your bingo card", subtitle: sub, card: card})
}

func (b *Bot) handleCall(ctx context.Context, i *discordgo.InteractionCreate, id string) {
	parts := parseIDArgs(id) // call:<cardID>
	if len(parts) != 2 {
		b.replyEphemeral(i, "Invalid button.")
		return
	}
	cardID, ok := atoi64(parts[1])
	if !ok {
		b.replyEphemeral(i, "Invalid button.")
		return
	}
	res, err := b.svc.CallBingo(ctx, i.GuildID, interactionUserID(i), cardID)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	// Finalize the card message: show the winning card read-only (no buttons); the
	// public win announcement is posted by the event bridge.
	b.updateCardView(i, cardView{
		title:    "🎉 BINGO!",
		subtitle: "You won " + res.Game.Instance.Label() + " - announced in the channel",
		card:     res.Card,
		readOnly: true,
	})
}

func (b *Bot) handleDealButton(ctx context.Context, i *discordgo.InteractionCreate, id string) {
	inst, err := bingo.ParseInstance(strings.TrimPrefix(id, "deal:"))
	if err != nil {
		b.replyEphemeral(i, "Invalid instance.")
		return
	}
	card, game, err := b.svc.DealCard(ctx, i.GuildID, interactionUserID(i), inst)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	b.respondCardView(i, cardView{
		title:    inst.Label(),
		subtitle: fmt.Sprintf("Game #%d - mark your squares", game.ID),
		card:     card,
	})
}

func (b *Bot) handleNewReplace(ctx context.Context, i *discordgo.InteractionCreate, id string) {
	inst, err := bingo.ParseInstance(strings.TrimPrefix(id, "newreplace:"))
	if err != nil {
		b.replyEphemeral(i, "Invalid instance.")
		return
	}
	poolIDs := b.allSharedPoolIDs(ctx, i.GuildID)
	game, err := b.svc.NewGame(ctx, i.GuildID, interactionUserID(i), inst, poolIDs, true)
	if err != nil {
		b.ackEphemeralEdit(i, b.describeError(err))
		return
	}
	b.respondEditText(i, fmt.Sprintf("Replaced the game. A fresh **%s** bingo (game #%d) is now open.", inst.Label(), game.ID))
	b.refreshStatusMessage(ctx, i.GuildID, inst)
	// Announcement is posted by the event bridge (onEvent) on the GameOpened event.
}

func (b *Bot) handleAbortConfirm(ctx context.Context, i *discordgo.InteractionCreate, id string) {
	inst, err := bingo.ParseInstance(strings.TrimPrefix(id, "abort:"))
	if err != nil {
		b.replyEphemeral(i, "Invalid instance.")
		return
	}
	if _, err := b.svc.AbortGame(ctx, i.GuildID, interactionUserID(i), inst); err != nil {
		b.respondEditText(i, b.describeError(err))
		return
	}
	b.respondEditText(i, fmt.Sprintf("Aborted the **%s** game. Its cards are now read-only.", inst.Label()))
	b.refreshStatusMessage(ctx, i.GuildID, inst)
}

func (b *Bot) handleInspectSelect(ctx context.Context, i *discordgo.InteractionCreate, id string) {
	if admin, err := b.svc.IsAdmin(ctx, i.GuildID, interactionUserID(i)); err != nil || !admin {
		b.replyEphemeral(i, "Only bingo admins can inspect cards.")
		return
	}
	values := i.MessageComponentData().Values
	if len(values) == 0 {
		b.replyEphemeral(i, "No card selected.")
		return
	}
	cardID, ok := atoi64(values[0])
	if !ok {
		b.replyEphemeral(i, "Invalid selection.")
		return
	}
	card, err := b.svc.Store().GetCard(ctx, i.GuildID, cardID)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	// Acknowledge the select without changing it, then show the card as a
	// separate ephemeral image so the picker stays usable.
	_ = b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	view := cardView{title: fmt.Sprintf("Card #%d", cardID), subtitle: "read-only", card: card, readOnly: true}
	data, err := view.responseData()
	if err != nil {
		return
	}
	b.followup(i, &discordgo.WebhookParams{
		Flags: discordgo.MessageFlagsEphemeral,
		Files: data.Files,
	})
}

func (b *Bot) handleClearPool(ctx context.Context, i *discordgo.InteractionCreate, id string) {
	poolID, ok := atoi64(strings.TrimPrefix(id, "clearpool:"))
	if !ok {
		b.respondEditText(i, "Invalid pool.")
		return
	}
	n, err := b.svc.ClearPool(ctx, i.GuildID, interactionUserID(i), poolID)
	if err != nil {
		b.respondEditText(i, b.describeError(err))
		return
	}
	b.respondEditText(i, fmt.Sprintf("Cleared %d squares from the pool.", n))
}

// respondEditText replaces an ephemeral component message with plain text and no
// components (used after a confirm button resolves).
func (b *Bot) respondEditText(i *discordgo.InteractionCreate, msg string) {
	empty := []discordgo.MessageComponent{}
	err := b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    msg,
			Flags:      discordgo.MessageFlagsEphemeral,
			Components: empty,
		},
	})
	if err != nil {
		b.log.Printf("edit text: %v", err)
	}
}

// followup posts an ephemeral follow-up message to an interaction.
func (b *Bot) followup(i *discordgo.InteractionCreate, params *discordgo.WebhookParams) {
	if _, err := b.session.FollowupMessageCreate(i.Interaction, true, params); err != nil {
		b.log.Printf("followup: %v", err)
	}
}
