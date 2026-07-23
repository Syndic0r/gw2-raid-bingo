package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
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
	case strings.HasPrefix(id, "newpick:"):
		b.handleNewPick(ctx, i, id)
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
		subtitle: "You won " + res.Game.Name + " - announced in the channel",
		card:     res.Card,
		readOnly: true,
	})
}

func (b *Bot) handleDealButton(ctx context.Context, i *discordgo.InteractionCreate, id string) {
	gameID, ok := atoi64(strings.TrimPrefix(id, "deal:"))
	if !ok {
		b.replyEphemeral(i, "Invalid game.")
		return
	}
	card, game, err := b.svc.DealCard(ctx, i.GuildID, interactionUserID(i), gameID)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	b.respondCardView(i, cardView{
		title:    game.Name,
		subtitle: "Mark your squares",
		card:     card,
	})
}

// handleNewPick receives the pool multi-select from /bingo new and opens the game
// from the chosen pools. The replace flag rides in the custom id ("newpick:<0|1>").
func (b *Bot) handleNewPick(ctx context.Context, i *discordgo.InteractionCreate, id string) {
	replace := strings.TrimPrefix(id, "newpick:") == "1"
	var poolIDs []int64
	for _, v := range i.MessageComponentData().Values {
		if pid, ok := atoi64(v); ok {
			poolIDs = append(poolIDs, pid)
		}
	}
	game, err := b.svc.NewGame(ctx, i.GuildID, interactionUserID(i), "", poolIDs, replace)
	if err != nil {
		b.respondEditText(i, b.describeError(err))
		return
	}
	b.respondEditText(i, fmt.Sprintf("Opened **%s** (game #%d). Players can join with `/bingo card`, or post a joinable message with `/bingo post`.", game.Name, game.ID))
	b.refreshStatusMessage(ctx, i.GuildID, game.ID)
	// The game-open announcement is posted by the event bridge (onEvent).
}

func (b *Bot) handleAbortConfirm(ctx context.Context, i *discordgo.InteractionCreate, id string) {
	gameID, ok := atoi64(strings.TrimPrefix(id, "abort:"))
	if !ok {
		b.replyEphemeral(i, "Invalid game.")
		return
	}
	game, err := b.svc.AbortGame(ctx, i.GuildID, interactionUserID(i), gameID)
	if err != nil {
		b.respondEditText(i, b.describeError(err))
		return
	}
	b.respondEditText(i, fmt.Sprintf("Aborted **%s**. Its cards are now read-only.", game.Name))
	b.refreshStatusMessage(ctx, i.GuildID, game.ID)
}

func (b *Bot) handleInspectSelect(ctx context.Context, i *discordgo.InteractionCreate, id string) {
	if !b.requireBingoAdmin(ctx, i, "Only bingo admins can inspect cards.") {
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
	if err := b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); err != nil {
		b.log.Printf("inspect ack: %v", err)
	}
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
