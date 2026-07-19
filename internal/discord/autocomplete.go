package discord

import (
	"context"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// routeAutocomplete answers slash-command autocomplete requests. Currently only
// the "pool" option is autocompleted, offering every pool that exists so an admin
// never has to guess a slug.
func (b *Bot) routeAutocomplete(ctx context.Context, i *discordgo.InteractionCreate) {
	focused := focusedOption(i.ApplicationCommandData().Options)
	if focused == nil || focused.Name != "pool" {
		b.respondAutocomplete(i, nil)
		return
	}
	b.respondAutocomplete(i, b.poolChoices(ctx, i.GuildID, strings.ToLower(focused.StringValue())))
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
