package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

func makeCard() store.Card {
	cells := make([]store.CardCell, bingo.CellCount)
	for i := range cells {
		cells[i] = store.CardCell{Index: i, Text: "sq", Marked: i == 0}
	}
	cells[bingo.CenterIdx] = store.CardCell{Index: bingo.CenterIdx, Text: bingo.FreeText, Free: true, Marked: true}
	return store.Card{ID: 7, Cells: cells}
}

func TestGridComponentsShape(t *testing.T) {
	rows := gridComponents(makeCard(), false)
	if len(rows) != bingo.Size {
		t.Fatalf("got %d rows, want %d", len(rows), bingo.Size)
	}
	total := 0
	var centerBtn discordgo.Button
	for _, row := range rows {
		ar, ok := row.(discordgo.ActionsRow)
		if !ok {
			t.Fatalf("row is not an ActionsRow: %T", row)
		}
		if len(ar.Components) != bingo.Size {
			t.Fatalf("row has %d buttons, want %d", len(ar.Components), bingo.Size)
		}
		for _, comp := range ar.Components {
			btn := comp.(discordgo.Button)
			total++
			if btn.CustomID == "" && !btn.Disabled {
				t.Error("non-free button missing custom id")
			}
		}
	}
	if total != bingo.CellCount {
		t.Fatalf("total buttons = %d, want %d", total, bingo.CellCount)
	}
	// The free centre button is disabled.
	centerRow := rows[bingo.CenterIdx/bingo.Size].(discordgo.ActionsRow)
	centerBtn = centerRow.Components[bingo.CenterIdx%bingo.Size].(discordgo.Button)
	if !centerBtn.Disabled {
		t.Error("free centre button should be disabled")
	}
}

func TestToggleCustomIDRoundTrip(t *testing.T) {
	rows := gridComponents(makeCard(), false)
	// Cell 0 is marked; its button should be Success style with a valid id.
	btn := rows[0].(discordgo.ActionsRow).Components[0].(discordgo.Button)
	if btn.Style != discordgo.SuccessButton {
		t.Errorf("marked cell style = %v, want Success", btn.Style)
	}
	parts := parseIDArgs(btn.CustomID)
	if len(parts) != 3 || parts[0] != "tog" {
		t.Fatalf("bad custom id %q", btn.CustomID)
	}
	cardID, ok := atoi64(parts[1])
	if !ok || cardID != 7 {
		t.Fatalf("card id parse: %q", parts[1])
	}
}
