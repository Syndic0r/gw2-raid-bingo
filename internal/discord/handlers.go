package discord

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
	"github.com/Syndic0r/gw2-raid-bingo/internal/service"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// routeCommand dispatches a slash command.
func (b *Bot) routeCommand(ctx context.Context, i *discordgo.InteractionCreate) {
	switch i.ApplicationCommandData().Name {
	case "setup":
		b.handleSetup(ctx, i)
	case "bingo":
		sub, opts := subcommand(i)
		switch sub {
		case "card":
			b.handleCard(ctx, i, opts)
		case "status":
			b.handleStatus(ctx, i, opts)
		case "new":
			b.handleNew(ctx, i, opts)
		case "abort":
			b.handleAbort(ctx, i, opts)
		case "post":
			b.handlePost(ctx, i, opts)
		case "cards":
			b.handleInspect(ctx, i, opts)
		case "schedule":
			b.handleSchedule(ctx, i, opts)
		case "scheduled":
			b.handleScheduled(ctx, i)
		case "unschedule":
			b.handleUnschedule(ctx, i, opts)
		default:
			b.replyEphemeral(i, "Unknown subcommand.")
		}
	case "bingo-data":
		b.handleData(ctx, i)
	}
}

// parseGameOpt reads and validates the "game" option (a game id from the
// autocomplete list).
func (b *Bot) parseGameOpt(i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) (int64, bool) {
	id, ok := atoi64(optString(opts, "game"))
	if !ok || id <= 0 {
		b.replyEphemeral(i, "Pick a game from the list (type to search).")
		return 0, false
	}
	return id, true
}

// poolSelectData builds an ephemeral pool multi-select response. Discord caps a
// select at 25 options, so a guild with more pools sees the first 25 (with a note).
func poolSelectData(customID, content string, pools []store.Pool) *discordgo.InteractionResponseData {
	options := make([]discordgo.SelectMenuOption, 0, 25)
	for _, p := range pools {
		options = append(options, discordgo.SelectMenuOption{Label: truncate(p.Name, 100), Value: strconv.FormatInt(p.ID, 10)})
		if len(options) == 25 {
			break
		}
	}
	if len(pools) > 25 {
		content += "\n(Showing the first 25 pools; use the website to pick from more.)"
	}
	minSel := 1
	return &discordgo.InteractionResponseData{
		Flags:   discordgo.MessageFlagsEphemeral,
		Content: content,
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType:  discordgo.StringSelectMenu,
					CustomID:  customID,
					MinValues: &minSel,
					MaxValues: len(options),
					Options:   options,
				},
			}},
		},
	}
}

func (b *Bot) handleNew(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	if !b.requireBingoAdmin(ctx, i, "Only bingo admins can open a game.") {
		return
	}
	pools, err := b.svc.Store().ListPools(ctx, i.GuildID)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	if len(pools) == 0 {
		b.replyEphemeral(i, "This server has no pools yet. Add squares with `/bingo-data add` (or `/bingo-data pool-add` to make a new pool) first.")
		return
	}
	flag := "0"
	if optBool(opts, "replace") {
		flag = "1"
	}
	b.respond(i, poolSelectData("newpick:"+flag,
		"Pick the pools to build this game from. A card needs 24 unique squares, so the pools you choose must have that many between them.", pools))
}

func (b *Bot) handleAbort(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameID, ok := b.parseGameOpt(i, opts)
	if !ok {
		return
	}
	game, err := b.svc.Store().GetGame(ctx, i.GuildID, gameID)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	// Confirm before aborting.
	b.respond(i, &discordgo.InteractionResponseData{
		Flags:   discordgo.MessageFlagsEphemeral,
		Content: fmt.Sprintf("Abort **%s**? All its cards become read-only.", game.Name),
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Abort game", Style: discordgo.DangerButton, CustomID: fmt.Sprintf("abort:%d", game.ID)},
			}},
		},
	})
}

func (b *Bot) handleCard(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameID, ok := b.parseGameOpt(i, opts)
	if !ok {
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

func (b *Bot) handleStatus(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameID, ok := b.parseGameOpt(i, opts)
	if !ok {
		return
	}
	stats, err := b.svc.GameStatsForGame(ctx, i.GuildID, gameID)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	b.respond(i, &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{b.statusEmbed(stats)}})
}

// handlePost creates the public status message for a game.
func (b *Bot) handlePost(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameID, ok := b.parseGameOpt(i, opts)
	if !ok {
		return
	}
	if !b.requireBingoAdmin(ctx, i, "Only bingo admins can post the game status message.") {
		return
	}
	stats, err := b.svc.GameStatsForGame(ctx, i.GuildID, gameID)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	msg, err := b.session.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{b.statusEmbed(stats)},
		Components: statusComponents(gameID),
	})
	if err != nil {
		b.replyEphemeral(i, "Could not post the status message.")
		return
	}
	if err := b.svc.Store().UpsertTrackedMessage(ctx, i.GuildID, gameID, store.MsgStatus, i.ChannelID, msg.ID); err != nil {
		b.log.Printf("track status message: %v", err)
	}
	b.replyEphemeral(i, "Posted a live status message here. It updates as players join and mark squares.")
}

// handleInspect lets an admin pick a player and view their card read-only.
func (b *Bot) handleInspect(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	gameID, ok := b.parseGameOpt(i, opts)
	if !ok {
		return
	}
	if !b.requireBingoAdmin(ctx, i, "Only bingo admins can inspect players' cards.") {
		return
	}
	stats, err := b.svc.GameStatsForGame(ctx, i.GuildID, gameID)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	if stats.PlayerCount == 0 {
		b.replyEphemeral(i, "No players have cards in this game yet.")
		return
	}
	// Offer a user select scoped to the players; the component renders the card.
	options := make([]discordgo.SelectMenuOption, 0, len(stats.Leaders))
	for _, p := range stats.Leaders {
		options = append(options, discordgo.SelectMenuOption{
			Label: fmt.Sprintf("Card #%d - %d marked, best line %d", p.CardID, p.Marked, p.BestLine),
			Value: fmt.Sprintf("%d", p.CardID),
		})
		if len(options) == 25 {
			break // Discord select-menu option cap
		}
	}
	b.respond(i, &discordgo.InteractionResponseData{
		Flags:   discordgo.MessageFlagsEphemeral,
		Content: fmt.Sprintf("Inspect a card for **%s** (read-only):", stats.Game.Name),
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType: discordgo.StringSelectMenu,
					CustomID: fmt.Sprintf("inspect:%d", gameID),
					Options:  options,
				},
			}},
		},
	})
}

// requireBingoAdmin verifies the acting user is a bingo admin, replying with a
// describeError on lookup failure or with deniedMsg when they are not, and
// returning false in both cases. It is the single admin gate shared by the data
// and game-management commands (the sibling ensureCanConfigure guards /setup).
func (b *Bot) requireBingoAdmin(ctx context.Context, i *discordgo.InteractionCreate, deniedMsg string) bool {
	admin, err := b.svc.IsAdmin(ctx, i.GuildID, interactionUserID(i))
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return false
	}
	if !admin {
		b.replyEphemeral(i, deniedMsg)
		return false
	}
	return true
}

// describeError maps internal errors to friendly, user-facing text.
func (b *Bot) describeError(err error) string {
	switch {
	case errors.Is(err, service.ErrForbidden):
		return "You do not have permission to do that."
	case errors.Is(err, service.ErrNoAnnounceChannel):
		return "Set an announcement channel first with `/setup` - that is where win celebrations are posted. Games can then be started from any channel."
	case errors.Is(err, bingo.ErrNotEnoughEntries):
		return fmt.Sprintf("The selected pools do not have enough squares yet (%d are needed). A bingo admin can add more with `/bingo-data add`.", bingo.FillCount)
	case errors.Is(err, store.ErrNoPoolsSelected):
		return "Select at least one pool to start a game."
	case errors.Is(err, store.ErrGameNotOpen):
		return "That game is not open."
	case errors.Is(err, store.ErrGameOpen):
		return "A game with those pools is already open. Re-run `/bingo new replace:true` to abort it and start fresh."
	case errors.Is(err, store.ErrNoBingo):
		return "That card does not have a completed line yet."
	case errors.Is(err, store.ErrNotFound):
		return "That game could not be found - it may have finished. A bingo admin can start one with `/bingo new`."
	case errors.Is(err, store.ErrValidation):
		return err.Error()
	default:
		b.log.Printf("unexpected error: %v", err)
		return "Something went wrong. Please try again."
	}
}
