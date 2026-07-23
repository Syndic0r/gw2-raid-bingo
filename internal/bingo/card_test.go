package bingo

import (
	"math/rand"
	"testing"
)

func entries(prefix string, n int) []Entry {
	out := make([]Entry, n)
	for i := 0; i < n; i++ {
		out[i] = Entry{ID: int64(len(prefix)*1000 + i + 1), Text: prefix}
	}
	return out
}

func newRand() *rand.Rand { return rand.New(rand.NewSource(42)) }

func checkWellFormed(t *testing.T, c *Card) {
	t.Helper()
	if len(c.Cells) != CellCount {
		t.Fatalf("card has %d cells, want %d", len(c.Cells), CellCount)
	}
	seenID := map[int64]bool{}
	for idx, cell := range c.Cells {
		if cell.Index != idx {
			t.Errorf("cell %d has Index %d", idx, cell.Index)
		}
		if idx == CenterIdx {
			if !cell.Free || cell.Text != FreeText || cell.EntryID != 0 {
				t.Errorf("centre cell not a proper free space: %+v", cell)
			}
			continue
		}
		if cell.Free {
			t.Errorf("cell %d unexpectedly free", idx)
		}
		if cell.EntryID == 0 {
			t.Errorf("cell %d has zero EntryID", idx)
		}
		if seenID[cell.EntryID] {
			t.Errorf("duplicate EntryID %d on card", cell.EntryID)
		}
		seenID[cell.EntryID] = true
	}
}

func TestGenerateCard_SamplesUnion(t *testing.T) {
	// More than a full card's worth of entries: 24 are sampled, one dropped.
	c, err := GenerateCard(entries("pool", 40), newRand())
	if err != nil {
		t.Fatal(err)
	}
	checkWellFormed(t, c)
	placed := 0
	for _, cell := range c.Cells {
		if !cell.Free {
			placed++
		}
	}
	if placed != FillCount {
		t.Fatalf("placed %d squares, want %d", placed, FillCount)
	}
}

func TestGenerateCard_ExactlyEnough(t *testing.T) {
	c, err := GenerateCard(entries("pool", FillCount), newRand())
	if err != nil {
		t.Fatal(err)
	}
	checkWellFormed(t, c)
}

func TestGenerateCard_NotEnough(t *testing.T) {
	_, err := GenerateCard(entries("pool", FillCount-1), newRand())
	if err != ErrNotEnoughEntries {
		t.Fatalf("got %v, want ErrNotEnoughEntries", err)
	}
}

func TestGenerateCard_DeduplicatesAcrossPools(t *testing.T) {
	// An entry present twice must be counted once; with the overlap removed there
	// are exactly 24 distinct entries.
	a := entries("pool", 23) // 23 distinct ids
	extra := Entry{ID: 999999, Text: "one more"}
	combined := append(append([]Entry{}, a...), extra, a[0]) // +1 unique, +1 duplicate
	if got := UsableEntryCount(combined); got != 24 {
		t.Fatalf("UsableEntryCount = %d, want 24 (overlap counted once)", got)
	}
	c, err := GenerateCard(combined, newRand())
	if err != nil {
		t.Fatal(err)
	}
	checkWellFormed(t, c)
}

func TestGenerateCard_Deterministic(t *testing.T) {
	pool := entries("pool", 30)
	a, _ := GenerateCard(pool, rand.New(rand.NewSource(7)))
	b, _ := GenerateCard(pool, rand.New(rand.NewSource(7)))
	for i := range a.Cells {
		if a.Cells[i] != b.Cells[i] {
			t.Fatalf("same seed produced different cards at %d", i)
		}
	}
}
