package discord

import (
	"github.com/bwmarrin/discordgo"
)

// gameRefOption is an open-game reference: an autocomplete field whose value is a
// game id. The autocomplete lists the guild's open games by name.
func gameRefOption() *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type:         discordgo.ApplicationCommandOptionString,
		Name:         "game",
		Description:  "Which open game (type to search)",
		Required:     true,
		Autocomplete: true,
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
					Description: "Get your bingo card for an open game",
					Options:     []*discordgo.ApplicationCommandOption{gameRefOption()},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "status",
					Description: "Show stats and the website link for a game",
					Options:     []*discordgo.ApplicationCommandOption{gameRefOption()},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "new",
					Description: "Open a new game - pick the pools to play (bingo admins)",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionBoolean,
							Name:        "replace",
							Description: "If a game with the same pools is open, abort it and start fresh",
							Required:    false,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "abort",
					Description: "Abort an open game (bingo admins)",
					Options:     []*discordgo.ApplicationCommandOption{gameRefOption()},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "post",
					Description: "Post a live game status message players can join from (bingo admins)",
					Options:     []*discordgo.ApplicationCommandOption{gameRefOption()},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "cards",
					Description: "Inspect players' cards for a game, read-only (bingo admins)",
					Options:     []*discordgo.ApplicationCommandOption{gameRefOption()},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "schedule",
					Description: "Schedule a game to open later - pick the pools (bingo admins)",
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
					Description: "Create a pool of squares to build games from",
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
					Description: "Remove a square - pick a pool, then search for the square",
					Options: []*discordgo.ApplicationCommandOption{
						poolRefOption(),
						{
							Type:         discordgo.ApplicationCommandOptionString,
							Name:         "square",
							Description:  "The square to remove (type to search within the pool)",
							Required:     true,
							Autocomplete: true,
						},
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

// poolRefOption is a pool reference by slug, validated server-side against the
// guild's pools. Autocomplete offers every pool the guild has.
func poolRefOption() *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type:         discordgo.ApplicationCommandOptionString,
		Name:         "pool",
		Description:  "Pick a pool (type to search)",
		Required:     true,
		Autocomplete: true,
	}
}
