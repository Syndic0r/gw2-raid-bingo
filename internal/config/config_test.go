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
	// BASE_PATH unset defaults to root, so the redirect URI has no prefix (this keeps
	// an auto-deploy that predates the cutover serving exactly as before).
	env := map[string]string{"BASE_URL": "https://example.org/"}
	c, _ := LoadFrom(func(k string) string { return env[k] })
	if c.BaseURL != "https://example.org" {
		t.Errorf("trailing slash not trimmed: %q", c.BaseURL)
	}
	if c.BasePath != "" {
		t.Errorf("BasePath default = %q, want \"\" (root)", c.BasePath)
	}
	if got := c.RedirectURI(); got != "https://example.org/auth/callback" {
		t.Errorf("redirect = %q", got)
	}
	// With BASE_PATH=/play (as production sets it) the prefix is carried.
	env = map[string]string{"BASE_URL": "https://example.org", "BASE_PATH": "/play"}
	c, _ = LoadFrom(func(k string) string { return env[k] })
	if got := c.RedirectURI(); got != "https://example.org/play/auth/callback" {
		t.Errorf("prefixed redirect = %q", got)
	}
}

func TestBasePathNormalization(t *testing.T) {
	cases := map[string]string{
		"":       "",      // unset -> root (no prefix)
		"play":   "/play", // no slashes -> leading slash added
		"/play/": "/play", // trailing slash trimmed
		"/game":  "/game", // custom prefix kept
		"/":      "",      // explicit root -> no prefix
	}
	for in, want := range cases {
		env := map[string]string{"BASE_PATH": in}
		// An unset var and an empty var are indistinguishable via os.Getenv, so the
		// empty-string case exercises the default just like a missing var would.
		if in == "" {
			env = map[string]string{}
		}
		c, _ := LoadFrom(func(k string) string { return env[k] })
		if c.BasePath != want {
			t.Errorf("BASE_PATH %q -> BasePath %q, want %q", in, c.BasePath, want)
		}
	}
}
