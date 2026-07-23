package service

import (
	"context"
	"fmt"

	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// All data operations are admin-gated here so neither the bot nor the web layer
// can edit a guild's card texts without the check.

// CreatePool creates a named pool (admin only).
func (s *Service) CreatePool(ctx context.Context, guildID, userID, slug, name string) (store.Pool, error) {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return store.Pool{}, err
	}
	return s.store.CreatePool(ctx, guildID, slug, name)
}

// DeletePool removes a pool and its entries (admin only). Every pool is deletable.
func (s *Service) DeletePool(ctx context.Context, guildID, userID string, poolID int64) error {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return err
	}
	return s.store.DeletePool(ctx, guildID, poolID)
}

// AddEntry adds a square to a pool (admin only).
func (s *Service) AddEntry(ctx context.Context, guildID, userID string, poolID int64, text string) (store.Entry, error) {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return store.Entry{}, err
	}
	return s.store.AddEntry(ctx, guildID, poolID, text)
}

// EditEntry updates a square's text (admin only).
func (s *Service) EditEntry(ctx context.Context, guildID, userID string, entryID int64, text string) error {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return err
	}
	return s.store.EditEntry(ctx, guildID, entryID, text)
}

// RemoveEntry soft-deletes a square (admin only).
func (s *Service) RemoveEntry(ctx context.Context, guildID, userID string, entryID int64) error {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return err
	}
	return s.store.SoftDeleteEntry(ctx, guildID, entryID)
}

// ClearPool removes every square from a pool (admin only), returning how many.
func (s *Service) ClearPool(ctx context.Context, guildID, userID string, poolID int64) (int64, error) {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return 0, err
	}
	return s.store.ClearPoolEntries(ctx, guildID, poolID)
}

// ImportResult reports the outcome of a bulk import.
type ImportResult struct {
	Inserted    int
	PoolsMade   int
	SkippedRows int
}

// ImportData bulk-adds entries from parsed import data (admin only). Unlike the
// home-guild seed it appends to whatever is there and never marks a seed guild, so
// any server can seed itself from an exported template. Both the `instance` and
// `shared` buckets map to ordinary pools keyed by slug, created on demand.
func (s *Service) ImportData(ctx context.Context, guildID, userID string, d store.SeedData) (ImportResult, error) {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return ImportResult{}, err
	}
	var res ImportResult
	importBucket := func(pools map[string][]string) error {
		for slug, texts := range pools {
			pool, err := s.store.GetPool(ctx, guildID, slug)
			if err == store.ErrNotFound {
				pool, err = s.store.CreatePool(ctx, guildID, slug, store.TitleCaseSlug(slug))
				if err == nil {
					res.PoolsMade++
				}
			}
			if err != nil {
				return fmt.Errorf("import pool %q: %w", slug, err)
			}
			s.importInto(ctx, guildID, pool.ID, texts, &res)
		}
		return nil
	}
	if err := importBucket(d.Instance); err != nil {
		return res, err
	}
	if err := importBucket(d.Shared); err != nil {
		return res, err
	}
	return res, nil
}

// importInto adds each text to a pool, counting inserts and skips. A row that
// fails validation (empty, too long, over the cap) is skipped, not fatal, so one
// bad line doesn't abort a large import.
func (s *Service) importInto(ctx context.Context, guildID string, poolID int64, texts []string, res *ImportResult) {
	for _, t := range texts {
		if _, err := s.store.AddEntry(ctx, guildID, poolID, t); err != nil {
			res.SkippedRows++
			continue
		}
		res.Inserted++
	}
}
