package discord

import (
	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
)

// instanceChoices builds the slash-command choices for the instance option.
func instanceChoices() []*discordgo.ApplicationCommandOptionChoice {
	out := make([]*discordgo.ApplicationCommandOptionChoice, 0, len(bingo.Instances()))
	for _, inst := range bingo.Instances() {
		out = append(out, &discordgo.ApplicationCommandOptionChoice{
			Name:  inst.Label(),
			Value: string(inst),
		})
	}
	return out
}

func instanceOption(required bool) *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionString,
		Name:        "instance",
		Description: "Which raid wing or encounter",
		Required:    required,
		Choices:     instanceChoices(),
	}
}

// commandDefs returns every slash command the bot registers.
func commandDefs() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "setup",
			Description: "Configure the announcement channel and bingo-admin roles (server admins only)",
		},
		{
			Name:        "bingo",
			Description: "Play GW2 raid bingo",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "card",
					Description: "Get your bingo card for the open game",
					Options:     []*discordgo.ApplicationCommandOption{instanceOption(true)},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "status",
					Description: "Show stats and the website link for a game",
					Options:     []*discordgo.ApplicationCommandOption{instanceOption(true)},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "new",
					Description: "Open a new game for an instance (bingo admins)",
					Options: []*discordgo.ApplicationCommandOption{
						instanceOption(true),
						{
							Type:        discordgo.ApplicationCommandOptionBoolean,
							Name:        "replace",
							Description: "If a game is already open, abort it and start fresh",
							Required:    false,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "abort",
					Description: "Abort the open game for an instance (bingo admins)",
					Options:     []*discordgo.ApplicationCommandOption{instanceOption(true)},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "post",
					Description: "Post a live game status message players can join from (bingo admins)",
					Options:     []*discordgo.ApplicationCommandOption{instanceOption(true)},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "cards",
					Description: "Inspect players' cards for a game, read-only (bingo admins)",
					Options:     []*discordgo.ApplicationCommandOption{instanceOption(true)},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "schedule",
					Description: "Schedule games to open later, for chosen instances (bingo admins)",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "in", Description: "Open after a delay, e.g. 2h30m, 90m, 1d", Required: false},
						{Type: discordgo.ApplicationCommandOptionString, Name: "at", Description: "Open at a date-time, e.g. 2026-07-20 20:00", Required: false},
						{Type: discordgo.ApplicationCommandOptionString, Name: "tz", Description: "Timezone for 'at', e.g. Europe/Berlin (default UTC)", Required: false},
						{Type: discordgo.ApplicationCommandOptionBoolean, Name: "replace", Description: "Replace any game open at that time", Required: false},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "scheduled",
					Description: "List upcoming scheduled games (bingo admins)",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "unschedule",
					Description: "Cancel a scheduled game by its id (bingo admins)",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionInteger, Name: "id", Description: "Schedule id from /bingo scheduled", Required: true},
					},
				},
			},
		},
		{
			Name:        "bingo-data",
			Description: "Manage this server's bingo card texts (bingo admins)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "pool-add",
					Description: "Create a shared pool of squares mixed into every card",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionString, Name: "slug", Description: "Short id, a-z 0-9 hyphens", Required: true},
						{Type: discordgo.ApplicationCommandOptionString, Name: "name", Description: "Display name", Required: true},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "add",
					Description: "Add a square to a pool",
					Options: []*discordgo.ApplicationCommandOption{
						poolRefOption(),
						{Type: discordgo.ApplicationCommandOptionString, Name: "text", Description: "The bingo square text", Required: true},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "list",
					Description: "List a pool's squares",
					Options:     []*discordgo.ApplicationCommandOption{poolRefOption()},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "remove",
					Description: "Remove a square by its id (from /bingo-data list)",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionInteger, Name: "entry_id", Description: "Square id", Required: true},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "clear",
					Description: "Remove ALL squares from a pool (pick from the dropdown)",
					Options:     []*discordgo.ApplicationCommandOption{poolRefOption()},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "import",
					Description: "Bulk import squares from an attached JSON file",
					Options: []*discordgo.ApplicationCommandOption{
						{Type: discordgo.ApplicationCommandOptionAttachment, Name: "file", Description: "JSON export file", Required: true},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "export",
					Description: "Export this server's squares as a JSON file",
				},
			},
		},
	}
}

// poolRefOption is a free-text pool reference: an instance key (w1..htcm) or a
// shared pool slug. It is validated server-side against the guild's pools.
func poolRefOption() *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type:         discordgo.ApplicationCommandOptionString,
		Name:         "pool",
		Description:  "Pick a pool - the static wings/encounters and your shared pools all appear",
		Required:     true,
		Autocomplete: true,
	}
}
