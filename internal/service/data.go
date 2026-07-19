package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// All data operations are admin-gated here so neither the bot nor the web layer
// can edit a guild's card texts without the check.

// CreateSharedPool creates a named shared pool (admin only).
func (s *Service) CreateSharedPool(ctx context.Context, guildID, userID, slug, name string) (store.Pool, error) {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return store.Pool{}, err
	}
	return s.store.CreateSharedPool(ctx, guildID, slug, name)
}

// DeleteSharedPool removes a shared pool and its entries (admin only).
func (s *Service) DeleteSharedPool(ctx context.Context, guildID, userID string, poolID int64) error {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return err
	}
	return s.store.DeleteSharedPool(ctx, guildID, poolID)
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
// home-guild seed it appends to whatever is there and never marks a seed guild,
// so any server can seed itself from an exported template. Instance keys must be
// valid instances; shared keys create shared pools on demand.
func (s *Service) ImportData(ctx context.Context, guildID, userID string, d store.SeedData) (ImportResult, error) {
	if err := s.requireAdmin(ctx, guildID, userID); err != nil {
		return ImportResult{}, err
	}
	var res ImportResult

	for key, texts := range d.Instance {
		inst, err := bingo.ParseInstance(key)
		if err != nil {
			return res, fmt.Errorf("import instance %q: %w", key, err)
		}
		pool, err := s.store.InstancePool(ctx, guildID, inst)
		if err != nil {
			return res, err
		}
		s.importInto(ctx, guildID, pool.ID, texts, &res)
	}

	for slug, texts := range d.Shared {
		pool, err := s.store.GetPool(ctx, guildID, store.KindShared, slug)
		if err == store.ErrNotFound {
			pool, err = s.store.CreateSharedPool(ctx, guildID, slug, seedName(slug))
			if err == nil {
				res.PoolsMade++
			}
		}
		if err != nil {
			return res, fmt.Errorf("import shared pool %q: %w", slug, err)
		}
		s.importInto(ctx, guildID, pool.ID, texts, &res)
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

// seedName turns a slug like "general" into a display name "General".
func seedName(slug string) string {
	if slug == "" {
		return slug
	}
	return strings.ToUpper(slug[:1]) + slug[1:]
}
