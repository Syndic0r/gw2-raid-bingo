package web

import (
	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
	"github.com/Syndic0r/gw2-raid-bingo/internal/service"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// gameJSON serializes a game for the API.
func gameJSON(g store.Game) map[string]any {
	return map[string]any{
		"id":         g.ID,
		"instance":   string(g.Instance),
		"label":      g.Instance.Label(),
		"status":     g.Status,
		"createdAt":  g.CreatedAt,
		"finishedAt": g.FinishedAt,
		"winnerId":   g.WinnerUserID,
	}
}

// cardJSON serializes a card and its cells.
func cardJSON(c store.Card) map[string]any {
	cells := make([]map[string]any, len(c.Cells))
	for i, cell := range c.Cells {
		cells[i] = map[string]any{
			"index":  cell.Index,
			"text":   cell.Text,
			"marked": cell.Marked,
			"free":   cell.Free,
		}
	}
	return map[string]any{
		"id":       c.ID,
		"userId":   c.UserID,
		"cells":    cells,
		"hasBingo": bingo.HasBingo(c.Marks()),
	}
}

// leadersJSON serializes the leaderboard.
func leadersJSON(leaders []service.PlayerProgress) []map[string]any {
	out := make([]map[string]any, 0, len(leaders))
	for _, p := range leaders {
		out = append(out, map[string]any{
			"userId":   p.UserID,
			"marked":   p.Marked,
			"bestLine": p.BestLine,
			"hasBingo": p.HasBingo,
		})
	}
	return out
}
