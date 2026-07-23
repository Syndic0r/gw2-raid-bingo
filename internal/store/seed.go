package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

// SeedData is the parsed shape of the private seed file (data/seed/entries.json).
// It is only ever applied to the configured home guild. The `instance` and `shared`
// buckets are kept for backward compatibility with the existing file; both now map
// to ordinary pools keyed by slug (the former wing keys w1..w8/htcm are just slugs).
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
// every boot of the home guild never duplicates squares. Both the `instance` and
// `shared` buckets map to ordinary pools keyed by slug, creating any that are
// missing (named from the slug).
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
	apply := func(pools map[string][]string) error {
		for slug, texts := range pools {
			pool, err := s.GetPool(ctx, guildID, slug)
			if errors.Is(err, ErrNotFound) {
				pool, err = s.CreatePool(ctx, guildID, slug, TitleCaseSlug(slug))
			}
			if err != nil {
				return fmt.Errorf("seed pool %q: %w", slug, err)
			}
			n, err := s.seedPoolIfEmpty(ctx, guildID, pool.ID, texts)
			if err != nil {
				return err
			}
			inserted += n
		}
		return nil
	}
	if err := apply(d.Instance); err != nil {
		return inserted, err
	}
	if err := apply(d.Shared); err != nil {
		return inserted, err
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
// auto-pool creation. Rune-aware so a non-ASCII first character is not mangled by
// byte slicing (slugs are ASCII today, but this keeps the helper safe for reuse).
func TitleCaseSlug(slug string) string {
	if slug == "" {
		return slug
	}
	r := []rune(slug)
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}
