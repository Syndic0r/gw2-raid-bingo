package bingo

import "testing"

// mark builds a marks slice with the given indices set true.
func mark(idxs ...int) []bool {
	m := make([]bool, CellCount)
	for _, i := range idxs {
		m[i] = true
	}
	return m
}

func TestLines_Shape(t *testing.T) {
	if len(Lines) != 2*Size+2 {
		t.Fatalf("got %d lines, want %d", len(Lines), 2*Size+2)
	}
	for _, line := range Lines {
		if len(line) != Size {
			t.Fatalf("line length %d, want %d", len(line), Size)
		}
	}
}

func TestHasBingo_FreeCentreCountsForMiddleLines(t *testing.T) {
	// Middle row 10..14 includes the free centre (12); marking the other four wins.
	if !HasBingo(mark(10, 11, 13, 14)) {
		t.Error("middle row with free centre should be a bingo")
	}
	// Middle column 2,7,12,17,22 likewise.
	if !HasBingo(mark(2, 7, 17, 22)) {
		t.Error("middle column with free centre should be a bingo")
	}
}

func TestHasBingo_Diagonal(t *testing.T) {
	// Main diagonal 0,6,12,18,24; centre free.
	if !HasBingo(mark(0, 6, 18, 24)) {
		t.Error("main diagonal should win via free centre")
	}
	// Anti-diagonal 4,8,12,16,20.
	if !HasBingo(mark(4, 8, 16, 20)) {
		t.Error("anti-diagonal should win via free centre")
	}
}

func TestHasBingo_EdgeRowNeedsAllFive(t *testing.T) {
	// Top row 0..4 does not include the centre: four is not enough.
	if HasBingo(mark(0, 1, 2, 3)) {
		t.Error("four of five on an edge row must not be a bingo")
	}
	if !HasBingo(mark(0, 1, 2, 3, 4)) {
		t.Error("full edge row should be a bingo")
	}
}

func TestHasBingo_EmptyIsNoWin(t *testing.T) {
	if HasBingo(mark()) {
		t.Error("a fresh card (only free centre) must not be a bingo")
	}
}

func TestCompletedLines(t *testing.T) {
	// Complete the top row (line 0) and left column (line Size).
	m := mark(0, 1, 2, 3, 4, 5, 10, 15, 20)
	done := CompletedLines(m)
	want := map[int]bool{0: true, Size: true}
	if len(done) != len(want) {
		t.Fatalf("got lines %v, want the two lines %v", done, want)
	}
	for _, li := range done {
		if !want[li] {
			t.Errorf("unexpected completed line %d", li)
		}
	}
}

func TestBestProgress(t *testing.T) {
	if got := BestProgress(mark()); got != 1 {
		t.Errorf("empty card best progress = %d, want 1 (free centre)", got)
	}
	// Four on an edge row -> 4; the free centre can't help that line.
	if got := BestProgress(mark(0, 1, 2, 3)); got != 4 {
		t.Errorf("best progress = %d, want 4", got)
	}
	if got := BestProgress(mark(0, 1, 2, 3, 4)); got != Size {
		t.Errorf("completed line best progress = %d, want %d", got, Size)
	}
}
