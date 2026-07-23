package render

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
)

func TestRenderCard(t *testing.T) {
	cells := make([]Cell, bingo.CellCount)
	for i := range cells {
		cells[i] = Cell{Text: "some bingo square text that wraps", Marked: i%3 == 0}
	}
	cells[bingo.CenterIdx] = Cell{Text: bingo.FreeText, Free: true, Marked: true}

	out, err := RenderCard(Options{Title: "Wing 1 Bingo", Subtitle: "12 players", Cells: cells})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("output is not valid PNG: %v", err)
	}
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		t.Fatal("rendered image has no area")
	}
}

func TestRenderCard_WrongCellCount(t *testing.T) {
	if _, err := RenderCard(Options{Cells: make([]Cell, 10)}); err == nil {
		t.Fatal("expected error for wrong cell count")
	}
}

// TestRenderCard_NonLatinDoesNotPanic guards the one place where "store UTF-8" is
// not enough: the PNG renderer. The bundled Go font lacks CJK/Arabic/emoji glyphs,
// so those render as blank boxes - but rendering must never panic or error on them.
func TestRenderCard_NonLatinDoesNotPanic(t *testing.T) {
	samples := []string{"漢字ビンゴ", "بينغو مربع", "Ёлки", "🎉🎲🔥", "café résumé", "combining é"}
	cells := make([]Cell, bingo.CellCount)
	for i := range cells {
		cells[i] = Cell{Text: samples[i%len(samples)], Marked: i%2 == 0}
	}
	cells[bingo.CenterIdx] = Cell{Text: bingo.FreeText, Free: true, Marked: true}

	out, err := RenderCard(Options{Title: "漢字 + بينغو 🎉", Subtitle: "国际 players", Cells: cells})
	if err != nil {
		t.Fatalf("render non-Latin card: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("non-Latin output is not valid PNG: %v", err)
	}
}
