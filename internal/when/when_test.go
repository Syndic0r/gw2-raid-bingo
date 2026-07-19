package when

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"2h":    2 * time.Hour,
		"90m":   90 * time.Minute,
		"1h30m": 90 * time.Minute,
		"1d":    24 * time.Hour,
		"1d6h":  30 * time.Hour,
		" 45m ": 45 * time.Minute,
	}
	for in, want := range cases {
		got, err := ParseDuration(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%q = %v, want %v", in, got, want)
		}
	}
	for _, bad := range []string{"", "0h", "-2h", "abc", "d", "2x"} {
		if _, err := ParseDuration(bad); err == nil {
			t.Errorf("%q should fail", bad)
		}
	}
}

func TestParseAt(t *testing.T) {
	got, err := ParseAt("2026-07-20 20:00", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 7, 20, 20, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Day-first European format.
	if _, err := ParseAt("20.07.2026 20:00", "UTC"); err != nil {
		t.Errorf("european format: %v", err)
	}
	if _, err := ParseAt("nonsense", "UTC"); err == nil {
		t.Error("expected parse failure")
	}
	if _, err := ParseAt("2026-07-20 20:00", "Not/AZone"); err == nil {
		t.Error("expected timezone failure")
	}
}

func TestResolve(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)

	got, err := Resolve(now, "2h", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(now.Add(2 * time.Hour)) {
		t.Errorf("relative resolve = %v", got)
	}

	if _, err := Resolve(now, "2h", "2026-07-20 20:00", ""); err == nil {
		t.Error("both in and at should error")
	}
	if _, err := Resolve(now, "", "", ""); err == nil {
		t.Error("neither should error")
	}
	if _, err := Resolve(now, "", "2020-01-01 00:00", ""); err == nil {
		t.Error("past should error")
	}
	if _, err := Resolve(now, "1000d", "", ""); err == nil {
		t.Error("beyond horizon should error")
	}
}
