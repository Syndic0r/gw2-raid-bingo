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

func TestGenerateCard_FillsFromShared(t *testing.T) {
	// Small instance pool (like w1) forces the fill to come from shared.
	inst := entries("inst", 2)
	shared := entries("shared", 40)
	c, err := GenerateCard(inst, shared, newRand())
	if err != nil {
		t.Fatal(err)
	}
	checkWellFormed(t, c)

	// Every instance entry must appear (themed squares are guaranteed).
	got := map[int64]bool{}
	for _, cell := range c.Cells {
		got[cell.EntryID] = true
	}
	for _, e := range inst {
		if !got[e.ID] {
			t.Errorf("instance entry %d missing from card", e.ID)
		}
	}
}

func TestGenerateCard_SamplesLargeInstance(t *testing.T) {
	// htcm has 25 entries but only 24 non-centre cells: one gets dropped.
	inst := entries("htcm", 25)
	c, err := GenerateCard(inst, nil, newRand())
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
	c, err := GenerateCard(entries("inst", 10), entries("shared", 14), newRand())
	if err != nil {
		t.Fatal(err)
	}
	checkWellFormed(t, c)
}

func TestGenerateCard_NotEnough(t *testing.T) {
	_, err := GenerateCard(entries("inst", 5), entries("shared", 10), newRand())
	if err != ErrNotEnoughEntries {
		t.Fatalf("got %v, want ErrNotEnoughEntries", err)
	}
}

func TestGenerateCard_DeduplicatesAcrossPools(t *testing.T) {
	// An entry present in both slices must not be placed twice, and with the
	// overlap removed there is no longer enough to fill a card.
	shared := entries("shared", 23)
	inst := append(entries("inst", 1), shared[0]) // shares one ID with shared
	if got := UsableEntryCount(inst, shared); got != 24 {
		t.Fatalf("UsableEntryCount = %d, want 24 (overlap counted once)", got)
	}
	c, err := GenerateCard(inst, shared, newRand())
	if err != nil {
		t.Fatal(err)
	}
	checkWellFormed(t, c)
}

func TestGenerateCard_Deterministic(t *testing.T) {
	inst, shared := entries("inst", 3), entries("shared", 30)
	a, _ := GenerateCard(inst, shared, rand.New(rand.NewSource(7)))
	b, _ := GenerateCard(inst, shared, rand.New(rand.NewSource(7)))
	for i := range a.Cells {
		if a.Cells[i] != b.Cells[i] {
			t.Fatalf("same seed produced different cards at %d", i)
		}
	}
}
