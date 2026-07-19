package bingo

// Lines are the twelve winning lines of a 5x5 board: five rows, five columns,
// and the two diagonals, each listed as its cell indices. The centre (CenterIdx)
// is free, so every line through it needs only its other four cells marked.
var Lines = buildLines()

func buildLines() [][]int {
	lines := make([][]int, 0, 2*Size+2)
	for r := 0; r < Size; r++ { // rows
		row := make([]int, Size)
		for c := 0; c < Size; c++ {
			row[c] = r*Size + c
		}
		lines = append(lines, row)
	}
	for c := 0; c < Size; c++ { // columns
		col := make([]int, Size)
		for r := 0; r < Size; r++ {
			col[r] = r*Size + c
		}
		lines = append(lines, col)
	}
	diag, anti := make([]int, Size), make([]int, Size)
	for i := 0; i < Size; i++ {
		diag[i] = i*Size + i
		anti[i] = i*Size + (Size - 1 - i)
	}
	lines = append(lines, diag, anti)
	return lines
}

// marksWithFree returns a copy of marks with the free centre forced on, so line
// checks treat the centre as always marked regardless of the stored value.
func marksWithFree(marks []bool) []bool {
	m := make([]bool, CellCount)
	copy(m, marks)
	m[CenterIdx] = true
	return m
}

// CompletedLines returns the indices (into Lines) of every fully-marked line.
// marks must have length CellCount; the free centre is always treated as marked.
func CompletedLines(marks []bool) []int {
	m := marksWithFree(marks)
	var done []int
	for li, line := range Lines {
		if allMarked(m, line) {
			done = append(done, li)
		}
	}
	return done
}

// HasBingo reports whether marks completes at least one line.
func HasBingo(marks []bool) bool {
	m := marksWithFree(marks)
	for _, line := range Lines {
		if allMarked(m, line) {
			return true
		}
	}
	return false
}

// BestProgress returns the highest number of marked cells on any single line
// (0-5), for "one line away" style stats. The free centre counts as marked.
func BestProgress(marks []bool) int {
	m := marksWithFree(marks)
	best := 0
	for _, line := range Lines {
		n := 0
		for _, idx := range line {
			if m[idx] {
				n++
			}
		}
		if n > best {
			best = n
		}
	}
	return best
}

func allMarked(m []bool, line []int) bool {
	for _, idx := range line {
		if !m[idx] {
			return false
		}
	}
	return true
}
