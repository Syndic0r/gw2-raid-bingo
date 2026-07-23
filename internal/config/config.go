// Package config loads runtime configuration from the environment. On the host
// the values come from a systemd EnvironmentFile (/etc/gw2-raid-bingo.conf,
// root-owned 0600); in development they come from the shell or a dev env file.
// Secrets live only in the environment, never in the repo or the database.
package config

import (
	"fmt"
	"os"
	"strings"
)

// DefaultSeedGuildID is the home guild that receives the private seed data. It
// can be overridden with SEED_GUILD_ID but defaults to the known home guild.
const DefaultSeedGuildID = "1098188107708371015"

// Config is the resolved runtime configuration.
type Config struct {
	// Discord.
	BotToken     string // DISCORD_BOT_TOKEN
	AppID        string // DISCORD_APP_ID (application/client id)
	ClientSecret string // DISCORD_CLIENT_SECRET (web OAuth; optional for bot-only)
	DevGuildID   string // DISCORD_DEV_GUILD_ID: register commands instantly to one guild in dev

	// Data.
	DBPath      string // DB_PATH
	SeedFile    string // SEED_FILE (path to entries.json; optional)
	SeedGuildID string // SEED_GUILD_ID

	// Web.
	HTTPAddr string // HTTP_ADDR, default 127.0.0.1:8771
	BaseURL  string // BASE_URL: the ORIGIN only, e.g. https://gw2-raid-bingo.duckdns.org (no path)
	BasePath string // BASE_PATH: the path prefix the game is mounted under, e.g. /play (unset/"/" = root). Production sets /play; the landing lives at / on the same origin.
	Version  string // build version (set by main from the ldflags-stamped var, not from env)
}

// Getenv is the environment lookup, overridable in tests.
type Getenv func(string) string

// Load resolves configuration using os.Getenv and applies defaults.
func Load() (Config, error) { return LoadFrom(os.Getenv) }

// LoadFrom resolves configuration using the provided lookup.
func LoadFrom(get Getenv) (Config, error) {
	c := Config{
		BotToken:     strings.TrimSpace(get("DISCORD_BOT_TOKEN")),
		AppID:        strings.TrimSpace(get("DISCORD_APP_ID")),
		ClientSecret: strings.TrimSpace(get("DISCORD_CLIENT_SECRET")),
		DevGuildID:   strings.TrimSpace(get("DISCORD_DEV_GUILD_ID")),
		DBPath:       strings.TrimSpace(get("DB_PATH")),
		SeedFile:     strings.TrimSpace(get("SEED_FILE")),
		SeedGuildID:  strings.TrimSpace(get("SEED_GUILD_ID")),
		HTTPAddr:     strings.TrimSpace(get("HTTP_ADDR")),
		BaseURL:      strings.TrimRight(strings.TrimSpace(get("BASE_URL")), "/"),
		BasePath:     normalizeBasePath(get("BASE_PATH")),
	}
	if c.SeedGuildID == "" {
		c.SeedGuildID = DefaultSeedGuildID
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = "127.0.0.1:8771"
	}
	if c.DBPath == "" {
		c.DBPath = "data/bingo.db"
	}
	return c, nil
}

// RequireBot verifies the fields the Discord bot needs to start.
func (c Config) RequireBot() error {
	var missing []string
	if c.BotToken == "" {
		missing = append(missing, "DISCORD_BOT_TOKEN")
	}
	if c.AppID == "" {
		missing = append(missing, "DISCORD_APP_ID")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	return nil
}

// RequireWeb verifies the fields the web OAuth flow needs.
func (c Config) RequireWeb() error {
	var missing []string
	if c.AppID == "" {
		missing = append(missing, "DISCORD_APP_ID")
	}
	if c.ClientSecret == "" {
		missing = append(missing, "DISCORD_CLIENT_SECRET")
	}
	if c.BaseURL == "" {
		missing = append(missing, "BASE_URL")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required web config: %s", strings.Join(missing, ", "))
	}
	return nil
}

// RedirectURI is the Discord OAuth callback URL derived from BaseURL + BasePath.
// It must match a redirect URI registered on the Discord application exactly, e.g.
// https://gw2-raid-bingo.duckdns.org/play/auth/callback.
func (c Config) RedirectURI() string {
	if c.BaseURL == "" {
		return ""
	}
	return c.BaseURL + c.BasePath + "/auth/callback"
}

// normalizeBasePath cleans BASE_PATH into either "" (mounted at the origin root)
// or a single-leading-slash, no-trailing-slash prefix like "/play". An UNSET value
// defaults to "" (root), so an existing deploy that has not set BASE_PATH keeps
// serving at the root until the operator sets BASE_PATH=/play as part of the nginx
// cutover - this makes an auto-deploy on merge a behavioral no-op. Production sets
// BASE_PATH=/play (the game lives under /play, the landing at /); "/" also means root.
func normalizeBasePath(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return ""
	}
	p = "/" + strings.Trim(p, "/")
	if p == "/" {
		return ""
	}
	return p
}
