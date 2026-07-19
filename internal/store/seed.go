package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
)

// SeedData is the parsed shape of the private seed file (data/seed/entries.json).
// It is only ever applied to the configured home guild.
type SeedData struct {
	Source   map[string]any      `json:"source"`
	Instance map[string][]string `json:"instance"`
	Shared   map[string][]string `json:"shared"`
}

// ParseSeed decodes seed JSON, rejecting unknown top-level shapes.
func ParseSeed(r io.Reader) (SeedData, error) {
	var d SeedData
	dec := json.NewDecoder(r)
	if err := dec.Decode(&d); err != nil {
		return SeedData{}, fmt.Errorf("parse seed: %w", err)
	}
	return d, nil
}

// LoadSeedFile reads and parses a seed file from disk.
func LoadSeedFile(path string) (SeedData, error) {
	f, err := os.Open(path)
	if err != nil {
		return SeedData{}, err
	}
	defer f.Close()
	return ParseSeed(f)
}

// ApplySeed populates guildID's pools from the seed data. It is idempotent: a
// pool that already holds any entries is left untouched, so applying the seed on
// every boot of the home guild never duplicates squares. Instance keys must be
// valid instances; shared keys become shared pools named from the key.
//
// It reports how many entries were inserted.
func (s *Store) ApplySeed(ctx context.Context, guildID string, d SeedData) (int, error) {
	if err := s.EnsureGuild(ctx, guildID); err != nil {
		return 0, err
	}
	if err := s.MarkSeedGuild(ctx, guildID); err != nil {
		return 0, err
	}

	inserted := 0

	for key, texts := range d.Instance {
		inst, err := bingo.ParseInstance(key)
		if err != nil {
			return inserted, fmt.Errorf("seed instance %q: %w", key, err)
		}
		pool, err := s.InstancePool(ctx, guildID, inst)
		if err != nil {
			return inserted, err
		}
		n, err := s.seedPoolIfEmpty(ctx, guildID, pool.ID, texts)
		if err != nil {
			return inserted, err
		}
		inserted += n
	}

	for slug, texts := range d.Shared {
		pool, err := s.GetPool(ctx, guildID, KindShared, slug)
		if err == ErrNotFound {
			pool, err = s.CreateSharedPool(ctx, guildID, slug, TitleCaseSlug(slug))
		}
		if err != nil {
			return inserted, fmt.Errorf("seed shared pool %q: %w", slug, err)
		}
		n, err := s.seedPoolIfEmpty(ctx, guildID, pool.ID, texts)
		if err != nil {
			return inserted, err
		}
		inserted += n
	}
	return inserted, nil
}

// seedPoolIfEmpty adds texts to a pool only when it currently has no active
// entries, keeping the seed idempotent.
func (s *Store) seedPoolIfEmpty(ctx context.Context, guildID string, poolID int64, texts []string) (int, error) {
	existing, err := s.ListEntries(ctx, guildID, poolID, true)
	if err != nil {
		return 0, err
	}
	if len(existing) > 0 {
		return 0, nil
	}
	n := 0
	for _, t := range texts {
		if strings.TrimSpace(t) == "" {
			continue
		}
		if _, err := s.AddEntry(ctx, guildID, poolID, t); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// TitleCaseSlug turns a slug like "general" into a display name "General". It is
// the single default-name derivation shared by seeding here and by the service's
// auto-pool creation.
func TitleCaseSlug(slug string) string {
	if slug == "" {
		return slug
	}
	return strings.ToUpper(slug[:1]) + slug[1:]
}
