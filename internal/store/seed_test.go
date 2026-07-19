package store

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
)

const seedJSON = `{
  "source": {"repo": "x", "commit": "y"},
  "instance": {"w1": ["vale guardian teleports"], "htcm": ["lightning struck", "greed death"]},
  "shared": {"general": ["line one", "line two", "  ", "line three"]}
}`

func TestApplySeed(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	d, err := ParseSeed(strings.NewReader(seedJSON))
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.ApplySeed(ctx, guild, d)
	if err != nil {
		t.Fatal(err)
	}
	// 1 (w1) + 2 (htcm) + 3 (general, blank skipped) = 6.
	if n != 6 {
		t.Fatalf("inserted %d, want 6", n)
	}

	// Applying again is a no-op (pools already populated).
	n2, err := s.ApplySeed(ctx, guild, d)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Fatalf("re-seed inserted %d, want 0", n2)
	}

	// The guild is flagged as a seed guild.
	gs, err := s.GetGuildSettings(ctx, guild)
	if err != nil {
		t.Fatal(err)
	}
	if !gs.IsSeedGuild {
		t.Error("guild not marked as seed guild")
	}

	// A shared pool named from the slug exists.
	pool, err := s.GetPool(ctx, guild, KindShared, "general")
	if err != nil {
		t.Fatal(err)
	}
	if pool.Name != "General" {
		t.Errorf("shared pool name = %q, want General", pool.Name)
	}
}

func TestApplySeed_RejectsUnknownInstance(t *testing.T) {
	s := newStore(t)
	d := SeedData{Instance: map[string][]string{"w99": {"bad"}}}
	if _, err := s.ApplySeed(context.Background(), guild, d); err == nil {
		t.Fatal("expected error for unknown instance key")
	}
}

// TestRealSeedFile guards against schema drift between the committed seed file
// and the loader. It is skipped when the file is not reachable from the test's
// working directory (e.g. an isolated CI checkout of only app/).
func TestRealSeedFile(t *testing.T) {
	path := filepath.Join("..", "..", "..", "data", "seed", "entries.json")
	if _, err := os.Stat(path); err != nil {
		t.Skip("seed file not present relative to test dir")
	}
	d, err := LoadSeedFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.ApplySeed(ctx, guild, d); err != nil {
		t.Fatal(err)
	}
	// htcm has 25 entries in the notebook; a full card must be dealable from it.
	game, err := s.NewGame(ctx, guild, bingo.HTCM, "host", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetOrDealCard(ctx, guild, game.ID, "u1", rand.New(rand.NewSource(1))); err != nil {
		t.Fatalf("deal from htcm seed: %v", err)
	}
}
