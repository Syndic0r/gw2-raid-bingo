package discord

import (
	"context"
	"errors"
	"fmt"

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

// parseInstanceOpt reads and validates the instance option.
func (b *Bot) parseInstanceOpt(i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) (bingo.Instance, bool) {
	inst, err := bingo.ParseInstance(optString(opts, "instance"))
	if err != nil {
		b.replyEphemeral(i, "That is not a valid instance.")
		return "", false
	}
	return inst, true
}

func (b *Bot) handleNew(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	inst, ok := b.parseInstanceOpt(i, opts)
	if !ok {
		return
	}
	replace := optBool(opts, "replace")
	poolIDs := b.allSharedPoolIDs(ctx, i.GuildID)

	game, err := b.svc.NewGame(ctx, i.GuildID, interactionUserID(i), inst, poolIDs, replace)
	if errors.Is(err, store.ErrGameOpen) {
		b.respond(i, &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: fmt.Sprintf("A game is already open for **%s**. Replacing it will abort the current game and make all its cards read-only.", inst.Label()),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{Components: []discordgo.MessageComponent{
					discordgo.Button{Label: "Replace it", Style: discordgo.DangerButton, CustomID: "newreplace:" + string(inst)},
				}},
			},
		})
		return
	}
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	b.replyEphemeralf(i, "Opened a new bingo game for **%s**. Players can run `/bingo card` to join. Post a joinable status message with `/bingo post`.", game.Instance.Label())
	b.refreshStatusMessage(ctx, i.GuildID, inst)
	// The game-open announcement is posted by the event bridge (onEvent), so it
	// fires for web- and scheduler-created games too, not just this command.
}

func (b *Bot) handleAbort(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	inst, ok := b.parseInstanceOpt(i, opts)
	if !ok {
		return
	}
	// Confirm before aborting.
	b.respond(i, &discordgo.InteractionResponseData{
		Flags:   discordgo.MessageFlagsEphemeral,
		Content: fmt.Sprintf("Abort the open game for **%s**? All its cards become read-only.", inst.Label()),
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Abort game", Style: discordgo.DangerButton, CustomID: "abort:" + string(inst)},
			}},
		},
	})
}

func (b *Bot) handleCard(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	inst, ok := b.parseInstanceOpt(i, opts)
	if !ok {
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

func (b *Bot) handleStatus(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	inst, ok := b.parseInstanceOpt(i, opts)
	if !ok {
		return
	}
	stats, err := b.svc.GameStatsForInstance(ctx, i.GuildID, inst)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	b.respond(i, &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{b.statusEmbed(stats)}})
}

// handlePost creates or refreshes the public status message for an instance.
func (b *Bot) handlePost(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	inst, ok := b.parseInstanceOpt(i, opts)
	if !ok {
		return
	}
	if admin, err := b.svc.IsAdmin(ctx, i.GuildID, interactionUserID(i)); err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	} else if !admin {
		b.replyEphemeral(i, "Only bingo admins can post the game status message.")
		return
	}
	stats, err := b.svc.GameStatsForInstance(ctx, i.GuildID, inst)
	if err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	}
	msg, err := b.session.ChannelMessageSendComplex(i.ChannelID, &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{b.statusEmbed(stats)},
		Components: statusComponents(inst),
	})
	if err != nil {
		b.replyEphemeral(i, "Could not post the status message.")
		return
	}
	if err := b.svc.Store().UpsertTrackedMessage(ctx, i.GuildID, inst, store.MsgStatus, i.ChannelID, msg.ID); err != nil {
		b.log.Printf("track status message: %v", err)
	}
	b.replyEphemeral(i, "Posted a live status message here. It updates as players join and mark squares.")
}

// handleInspect lets an admin pick a player and view their card read-only.
func (b *Bot) handleInspect(ctx context.Context, i *discordgo.InteractionCreate, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	inst, ok := b.parseInstanceOpt(i, opts)
	if !ok {
		return
	}
	if admin, err := b.svc.IsAdmin(ctx, i.GuildID, interactionUserID(i)); err != nil {
		b.replyEphemeral(i, b.describeError(err))
		return
	} else if !admin {
		b.replyEphemeral(i, "Only bingo admins can inspect players' cards.")
		return
	}
	stats, err := b.svc.GameStatsForInstance(ctx, i.GuildID, inst)
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
		Content: fmt.Sprintf("Inspect a card for **%s** (read-only):", inst.Label()),
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					MenuType: discordgo.StringSelectMenu,
					CustomID: "inspect:" + string(inst),
					Options:  options,
				},
			}},
		},
	})
}

// describeError maps internal errors to friendly, user-facing text.
func (b *Bot) describeError(err error) string {
	switch {
	case errors.Is(err, service.ErrForbidden):
		return "You do not have permission to do that."
	case errors.Is(err, service.ErrNoAnnounceChannel):
		return "Set an announcement channel first with `/setup` - that is where win celebrations are posted. Games can then be started from any channel."
	case errors.Is(err, bingo.ErrNotEnoughEntries):
		return "This instance does not have enough squares yet (24 are needed). A bingo admin can add more with `/bingo-data add`."
	case errors.Is(err, store.ErrGameNotOpen):
		return "That game is not open."
	case errors.Is(err, store.ErrGameOpen):
		return "A game is already open for this instance."
	case errors.Is(err, store.ErrNoBingo):
		return "That card does not have a completed line yet."
	case errors.Is(err, store.ErrNotFound):
		return "There is no open game for this instance. A bingo admin can start one with `/bingo new`."
	case errors.Is(err, store.ErrValidation):
		return err.Error()
	default:
		b.log.Printf("unexpected error: %v", err)
		return "Something went wrong. Please try again."
	}
}
