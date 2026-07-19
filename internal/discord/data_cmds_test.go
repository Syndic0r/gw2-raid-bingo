package discord

import "testing"

func TestIsDiscordCDNURL(t *testing.T) {
	ok := []string{
		"https://cdn.discordapp.com/attachments/1/2/file.json",
		"https://media.discordapp.net/attachments/1/2/file.json",
		"https://CDN.DISCORDAPP.COM/a/b/c.json",
	}
	for _, u := range ok {
		if !isDiscordCDNURL(u) {
			t.Errorf("%q should be allowed", u)
		}
	}
	bad := []string{
		"http://cdn.discordapp.com/a.json",              // not https
		"https://evil.example.com/a.json",               // wrong host
		"https://cdn.discordapp.com.evil.example/a",     // suffix trick
		"https://user@cdn.discordapp.com@evil.example/", // userinfo trick
		"://bad",
		"",
	}
	for _, u := range bad {
		if isDiscordCDNURL(u) {
			t.Errorf("%q must be rejected", u)
		}
	}
}

func TestTruncateRuneSafe(t *testing.T) {
	s := "ééééé" // 5 runes, 10 bytes
	got := truncate(s, 3)
	if got != "éé…" {
		t.Errorf("truncate = %q, want éé…", got)
	}
	if truncate("short", 100) != "short" {
		t.Error("short strings must be unchanged")
	}
}
