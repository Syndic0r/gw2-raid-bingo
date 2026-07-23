package discord

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"
)

// routeAutocomplete answers slash-command autocomplete requests: the "game" option
// lists a guild's open games, and "pool"/"square" help pick pools and their squares.
func (b *Bot) routeAutocomplete(ctx context.Context, i *discordgo.InteractionCreate) {
	focused := focusedOption(i.ApplicationCommandData().Options)
	if focused == nil {
		b.respondAutocomplete(i, nil)
		return
	}
	typed := strings.ToLower(focused.StringValue())
	switch focused.Name {
	case "game":
		b.respondAutocomplete(i, b.gameChoices(ctx, i.GuildID, typed))
	case "pool":
		b.respondAutocomplete(i, b.poolChoices(ctx, i.GuildID, typed))
	case "square":
		// Search the squares within the pool the user already picked.
		_, opts := subcommand(i)
		b.respondAutocomplete(i, b.entryChoices(ctx, i.GuildID, optString(opts, "pool"), typed))
	default:
		b.respondAutocomplete(i, nil)
	}
}

// gameChoices lists the guild's open games filtered by the typed text; each
// choice's value is the game id.
func (b *Bot) gameChoices(ctx context.Context, guildID, typed string) []*discordgo.ApplicationCommandOptionChoice {
	games, err := b.svc.Store().ListOpenGames(ctx, guildID)
	if err != nil {
		return nil
	}
	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, g := range games {
		if typed != "" && !strings.Contains(strings.ToLower(g.Name), typed) {
			continue
		}
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  truncate(g.Name, 100),
			Value: strconv.FormatInt(g.ID, 10),
		})
		if len(choices) >= 25 {
			break
		}
	}
	return choices
}

// entryChoices lists the squares in a pool, filtered by the typed text; each
// choice's value is the entry id, so removing needs no numeric guessing.
func (b *Bot) entryChoices(ctx context.Context, guildID, poolRef, typed string) []*discordgo.ApplicationCommandOptionChoice {
	if strings.TrimSpace(poolRef) == "" {
		return nil // no pool chosen yet
	}
	pool, err := b.resolvePool(ctx, guildID, poolRef)
	if err != nil {
		return nil
	}
	entries, err := b.svc.Store().ListEntries(ctx, guildID, pool.ID, true)
	if err != nil {
		return nil
	}
	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, e := range entries {
		if typed != "" && !strings.Contains(strings.ToLower(e.Text), typed) {
			continue
		}
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  truncate(e.Text, 100),
			Value: strconv.FormatInt(e.ID, 10),
		})
		if len(choices) >= 25 {
			break
		}
	}
	return choices
}

// focusedOption finds the option the user is currently typing into, searching
// into the subcommand's options.
func focusedOption(opts []*discordgo.ApplicationCommandInteractionDataOption) *discordgo.ApplicationCommandInteractionDataOption {
	for _, o := range opts {
		if o.Focused {
			return o
		}
		if len(o.Options) > 0 {
			if f := focusedOption(o.Options); f != nil {
				return f
			}
		}
	}
	return nil
}

// poolChoices lists the guild's pools filtered by the typed text; each choice's
// value is the pool slug.
func (b *Bot) poolChoices(ctx context.Context, guildID, typed string) []*discordgo.ApplicationCommandOptionChoice {
	pools, err := b.svc.Store().ListPools(ctx, guildID)
	if err != nil {
		return nil
	}
	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, p := range pools {
		if typed != "" && !strings.Contains(strings.ToLower(p.Slug), typed) && !strings.Contains(strings.ToLower(p.Name), typed) {
			continue
		}
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: truncate(p.Name, 100), Value: p.Slug})
		if len(choices) >= 25 {
			break
		}
	}
	return choices
}

func (b *Bot) respondAutocomplete(i *discordgo.InteractionCreate, choices []*discordgo.ApplicationCommandOptionChoice) {
	err := b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{Choices: choices},
	})
	if err != nil {
		b.log.Printf("autocomplete: %v", err)
	}
}

// truncate shortens s to at most n runes (not bytes), so a multi-byte character
// is never split into invalid UTF-8, which Discord would reject.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n-1]) + "…"
}
