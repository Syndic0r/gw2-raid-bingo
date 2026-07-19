package store

import "github.com/Syndic0r/gw2-raid-bingo/internal/bingo"

// Pool kinds.
const (
	KindInstance = "instance"
	KindShared   = "shared"
)

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

// Pool is a set of bingo squares: one of the nine fixed instance pools, or a
// named shared pool.
type Pool struct {
	ID        int64
	GuildID   string
	Kind      string
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

// Game is one bingo round for an instance.
type Game struct {
	ID           int64
	GuildID      string
	Instance     bingo.Instance
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
