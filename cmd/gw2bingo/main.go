// gw2bingo is the GW2 Raid Bingo service: Discord bot and web app in one binary.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Syndic0r/gw2-raid-bingo/internal/config"
	"github.com/Syndic0r/gw2-raid-bingo/internal/discord"
	"github.com/Syndic0r/gw2-raid-bingo/internal/events"
	"github.com/Syndic0r/gw2-raid-bingo/internal/service"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
	"github.com/Syndic0r/gw2-raid-bingo/internal/web"
)

// version is stamped at build time via -ldflags "-X main.version=vX.Y.Z".
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("gw2bingo", version)
		return
	}
	if err := run(); err != nil {
		log.Fatalf("gw2bingo: %v", err)
	}
}

func run() error {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	// Seed the home guild from the private seed file, if configured.
	if cfg.SeedFile != "" {
		if seed, err := store.LoadSeedFile(cfg.SeedFile); err != nil {
			logger.Printf("seed: could not load %s: %v", cfg.SeedFile, err)
		} else if n, err := st.ApplySeed(ctx, cfg.SeedGuildID, seed); err != nil {
			logger.Printf("seed: apply failed: %v", err)
		} else if n > 0 {
			logger.Printf("seed: inserted %d squares into home guild %s", n, cfg.SeedGuildID)
		}
	}

	hub := events.NewHub()

	if err := cfg.RequireBot(); err != nil {
		return err
	}
	bot, err := discord.New(cfg, hub, logger)
	if err != nil {
		return err
	}
	// The bot supplies the role resolver; build the service with it, then hand
	// the service back to the bot.
	svc := service.New(st, hub, bot.Resolver())
	bot.SetService(svc)

	if err := bot.Run(ctx); err != nil {
		return err
	}
	defer bot.Close()

	// The web server shares the store, service, and hub with the bot, and reads
	// the bot's guild membership for the picker. It runs unless web config is
	// absent, so the bot can still run standalone in development.
	if err := cfg.RequireWeb(); err != nil {
		logger.Printf("web: disabled (%v)", err)
	} else {
		srv := web.NewServer(cfg, svc, hub, bot, logger)
		go func() {
			if err := srv.Run(ctx); err != nil {
				logger.Printf("web: %v", err)
			}
		}()
	}

	logger.Printf("gw2bingo %s running (db=%s)", version, cfg.DBPath)
	<-ctx.Done()
	logger.Printf("shutting down")
	return nil
}
