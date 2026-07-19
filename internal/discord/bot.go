// Package discord is the Discord bot: it registers the slash commands and
// routes interactions (commands and button/select components) to the shared
// service layer. It holds no game logic of its own - authorization, state
// changes, and event publishing all live in the service.
package discord

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/config"
	"github.com/Syndic0r/gw2-raid-bingo/internal/events"
	"github.com/Syndic0r/gw2-raid-bingo/internal/service"
)

// Bot is the running Discord bot.
type Bot struct {
	cfg      config.Config
	session  *discordgo.Session
	svc      *service.Service
	hub      *events.Hub
	resolver *resolver
	log      *log.Logger
}

// New builds a bot. Because the service needs the bot's role resolver and the
// bot needs the service, construction is two-step: New creates the bot (and its
// resolver), the caller builds the service with Resolver(), then calls
// SetService before Run.
func New(cfg config.Config, hub *events.Hub, logger *log.Logger) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("discord session: %w", err)
	}
	// No privileged intents: roles are fetched via REST, so the bot needs only
	// guild-scoped interactions. This keeps verification simple as it scales.
	session.Identify.Intents = discordgo.IntentsGuilds

	b := &Bot{
		cfg:      cfg,
		session:  session,
		hub:      hub,
		resolver: newResolver(session),
		log:      logger,
	}
	session.AddHandler(b.onInteraction)
	session.AddHandler(b.onReady)
	return b, nil
}

// Resolver returns the bot's role resolver so the caller can build the service
// with it before starting.
func (b *Bot) Resolver() service.RoleResolver { return b.resolver }

// SetService attaches the shared service. It must be called before Run.
func (b *Bot) SetService(svc *service.Service) { b.svc = svc }

func (b *Bot) onReady(s *discordgo.Session, r *discordgo.Ready) {
	b.log.Printf("discord: connected as %s#%s", r.User.Username, r.User.Discriminator)
}

// Run opens the gateway connection and registers commands. It returns once the
// connection is established; call Close to shut down.
func (b *Bot) Run(ctx context.Context) error {
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("open gateway: %w", err)
	}
	if err := b.registerCommands(); err != nil {
		b.session.Close()
		return err
	}
	b.startEventBridge(ctx)
	b.startScheduler(ctx, 30*time.Second)
	return nil
}

// Close shuts the gateway connection down.
func (b *Bot) Close() error { return b.session.Close() }

// registerCommands overwrites the application's commands. In development a
// DevGuildID registers them to one guild (instant); otherwise they register
// globally (which Discord may take up to an hour to propagate).
func (b *Bot) registerCommands() error {
	guildID := b.cfg.DevGuildID
	_, err := b.session.ApplicationCommandBulkOverwrite(b.cfg.AppID, guildID, commandDefs())
	if err != nil {
		return fmt.Errorf("register commands: %w", err)
	}
	scope := "globally"
	if guildID != "" {
		scope = "to dev guild " + guildID
	}
	b.log.Printf("discord: registered %d commands %s", len(commandDefs()), scope)
	return nil
}

// onInteraction is the top-level router.
func (b *Bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Every interaction we handle happens inside a guild.
	if i.GuildID == "" {
		b.replyEphemeral(i, "This bot only works inside a server.")
		return
	}
	ctx := context.Background()
	// Make sure the guild exists in our store before any action.
	if err := b.svc.Store().EnsureGuild(ctx, i.GuildID); err != nil {
		b.log.Printf("ensure guild %s: %v", i.GuildID, err)
	}

	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		b.routeCommand(ctx, i)
	case discordgo.InteractionMessageComponent:
		b.routeComponent(ctx, i)
	case discordgo.InteractionApplicationCommandAutocomplete:
		b.routeAutocomplete(ctx, i)
	}
}
