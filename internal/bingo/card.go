package bingo

import (
	"errors"
	"math/rand"
)

// Grid geometry. A card is a 5x5 grid; the centre cell is a pre-marked free
// space, leaving 24 cells to fill from the entry pools.
const (
	Size      = 5
	CellCount = Size * Size   // 25
	CenterIdx = CellCount / 2 // 12
	FillCount = CellCount - 1 // 24 non-centre cells
	FreeText  = "Free!"
)

// ErrNotEnoughEntries is returned by GenerateCard when the selected pools together
// offer fewer than FillCount distinct entries, so a full card cannot be dealt.
var ErrNotEnoughEntries = errors.New("not enough entries to fill a card")

// Entry is one candidate square drawn from a pool. ID is the store's primary key
// (0 for the synthetic free centre); Text is the square's text at deal time.
type Entry struct {
	ID   int64
	Text string
}

// Cell is one square of a generated card. EntryID is 0 for the free centre.
// Text is snapshotted at generation time so later edits to the source entry
// never rewrite an in-progress or historical card.
type Cell struct {
	Index   int
	EntryID int64
	Text    string
	Free    bool
}

// Card is a generated 5x5 board: exactly CellCount cells, index CenterIdx free.
type Card struct {
	Cells []Cell
}

// GenerateCard deals a card for one player from the union of the selected pools'
// entries. The entries are de-duplicated by ID (so overlapping pools cannot place
// the same square twice), FillCount of them are sampled uniformly at random, and
// the chosen squares are shuffled into a random layout with the free centre at
// CenterIdx. It returns ErrNotEnoughEntries if fewer than FillCount distinct
// entries are available. r supplies all randomness, making generation
// deterministic and unit-testable.
func GenerateCard(entries []Entry, r *rand.Rand) (*Card, error) {
	pool := dedupe(entries, nil)
	if len(pool) < FillCount {
		return nil, ErrNotEnoughEntries
	}
	chosen := sample(pool, FillCount, r)

	r.Shuffle(len(chosen), func(i, j int) { chosen[i], chosen[j] = chosen[j], chosen[i] })

	cells := make([]Cell, 0, CellCount)
	next := 0 // index into chosen
	for idx := 0; idx < CellCount; idx++ {
		if idx == CenterIdx {
			cells = append(cells, Cell{Index: idx, Text: FreeText, Free: true})
			continue
		}
		e := chosen[next]
		next++
		cells = append(cells, Cell{Index: idx, EntryID: e.ID, Text: e.Text})
	}
	return &Card{Cells: cells}, nil
}

// UsableEntryCount reports how many distinct entries the selected pools jointly
// provide, so callers can pre-check whether a card can be dealt (>= FillCount) and
// give a clear "add more entries" message otherwise.
func UsableEntryCount(entries []Entry) int {
	return len(dedupe(entries, nil))
}

// dedupe returns the entries whose IDs are unique and not already in exclude,
// preserving input order. exclude may be nil.
func dedupe(entries []Entry, exclude map[int64]struct{}) []Entry {
	seen := make(map[int64]struct{}, len(entries))
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if _, skip := exclude[e.ID]; skip {
			continue
		}
		if _, dup := seen[e.ID]; dup {
			continue
		}
		seen[e.ID] = struct{}{}
		out = append(out, e)
	}
	return out
}

// sample returns n entries chosen uniformly at random without replacement.
// It assumes len(entries) >= n (callers guarantee this).
func sample(entries []Entry, n int, r *rand.Rand) []Entry {
	pool := make([]Entry, len(entries))
	copy(pool, entries)
	r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	return pool[:n]
}
