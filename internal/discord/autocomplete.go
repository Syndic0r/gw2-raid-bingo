package discord

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// routeAutocomplete answers slash-command autocomplete requests. Currently only
// the "pool" option is autocompleted, offering every pool that exists so an admin
// never has to guess a slug.
func (b *Bot) routeAutocomplete(ctx context.Context, i *discordgo.InteractionCreate) {
	focused := focusedOption(i.ApplicationCommandData().Options)
	if focused == nil {
		b.respondAutocomplete(i, nil)
		return
	}
	typed := strings.ToLower(focused.StringValue())
	switch focused.Name {
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

// poolChoices lists the guild's pools filtered by the typed text: the nine static
// wings/encounters first, then the guild's shared pools, each labeled by type.
func (b *Bot) poolChoices(ctx context.Context, guildID, typed string) []*discordgo.ApplicationCommandOptionChoice {
	var choices []*discordgo.ApplicationCommandOptionChoice
	add := func(value, label string) {
		if len(choices) >= 25 {
			return
		}
		if typed == "" || strings.Contains(strings.ToLower(value), typed) || strings.Contains(strings.ToLower(label), typed) {
			choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: truncate(label, 100), Value: value})
		}
	}

	for _, inst := range bingo.Instances() {
		add(string(inst), "Static · "+inst.Label())
	}
	pools, err := b.svc.Store().ListPools(ctx, guildID, store.KindShared)
	if err == nil {
		for _, p := range pools {
			add(p.Slug, "Shared · "+p.Name)
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
