package store

import "github.com/Syndic0r/gw2-raid-bingo/internal/bingo"

// poolKind is the value written to the vestigial pools.kind column. The former
// instance/shared distinction was removed (migration 0006): every pool is an
// ordinary, deletable pool now. The column and its CHECK remain only because
// rewriting the pools table with foreign keys on would cascade-delete entries.
const poolKind = "shared"

// Game statuses.
const (
	StatusOpen     = "open"
	StatusFinished = "finished"
	StatusAborted  = "aborted"
)

// GuildSettings is a guild's configuration row.
type GuildSettings struct {
	GuildID           string
	AnnounceChannelID string
	ParticipantRoleID string // pinged when a game is won; empty means no ping
	IsSeedGuild       bool
	CreatedAt         int64
	UpdatedAt         int64
}

// Pool is a named set of bingo squares. All pools are equal: a game is built from
// whichever pools the admin selects (there is no longer a privileged per-wing pool).
type Pool struct {
	ID        int64
	GuildID   string
	Slug      string
	Name      string
	CreatedAt int64
}

// Entry is one bingo square in a pool.
type Entry struct {
	ID        int64
	GuildID   string
	PoolID    int64
	Text      string
	Active    bool
	CreatedAt int64
	UpdatedAt int64
}

// Game is one bingo round, identified by the set of pools it draws from. Name is
// a human label (auto-derived from the pool names, or an admin-supplied override).
// PoolSetKey is the canonical key of the sorted, de-duped PoolIDs; at most one open
// game per (guild, PoolSetKey).
type Game struct {
	ID           int64
	GuildID      string
	Name         string
	PoolSetKey   string
	Status       string
	CreatedBy    string
	PoolIDs      []int64
	CreatedAt    int64
	FinishedAt   int64 // 0 while open
	WinnerUserID string
	WinnerCardID int64
}

// Card is one player's card, optionally with its cells populated.
type Card struct {
	ID        int64
	GameID    int64
	GuildID   string
	UserID    string
	CreatedAt int64
	Cells     []CardCell
}

// CardCell is one square of a stored card.
type CardCell struct {
	Index   int
	EntryID int64 // 0 for the free centre
	Text    string
	Free    bool
	Marked  bool
}

// Marks returns the cell marks as a bingo-package slice for win detection.
func (c *Card) Marks() []bool {
	m := make([]bool, bingo.CellCount)
	for _, cell := range c.Cells {
		if cell.Index >= 0 && cell.Index < bingo.CellCount {
			m[cell.Index] = cell.Marked
		}
	}
	return m
}
