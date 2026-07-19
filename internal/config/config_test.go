package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	c, err := LoadFrom(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if c.SeedGuildID != DefaultSeedGuildID {
		t.Errorf("seed guild = %q, want default", c.SeedGuildID)
	}
	if c.HTTPAddr != "127.0.0.1:8771" {
		t.Errorf("http addr = %q", c.HTTPAddr)
	}
	if c.DBPath == "" {
		t.Error("db path should default")
	}
}

func TestRequireBot(t *testing.T) {
	c, _ := LoadFrom(func(string) string { return "" })
	if err := c.RequireBot(); err == nil {
		t.Error("expected missing bot config error")
	}
	env := map[string]string{"DISCORD_BOT_TOKEN": "t", "DISCORD_APP_ID": "a"}
	c, _ = LoadFrom(func(k string) string { return env[k] })
	if err := c.RequireBot(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRedirectURI(t *testing.T) {
	env := map[string]string{"BASE_URL": "https://example.org/"}
	c, _ := LoadFrom(func(k string) string { return env[k] })
	if c.BaseURL != "https://example.org" {
		t.Errorf("trailing slash not trimmed: %q", c.BaseURL)
	}
	if got := c.RedirectURI(); got != "https://example.org/auth/callback" {
		t.Errorf("redirect = %q", got)
	}
}
