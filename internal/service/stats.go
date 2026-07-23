package service

import (
	"context"
	"sort"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// PlayerProgress is one player's standing in a game.
type PlayerProgress struct {
	UserID   string
	CardID   int64
	Marked   int // marked squares excluding the free centre
	BestLine int // most squares on a single line (0-5), free centre counted
	HasBingo bool
}

// GameStats summarizes an open (or finished) game for status displays.
type GameStats struct {
	Game        store.Game
	PlayerCount int
	Leaders     []PlayerProgress // sorted best-first
}

// GameStatsForGame computes stats for a specific game.
func (s *Service) GameStatsForGame(ctx context.Context, guildID string, gameID int64) (GameStats, error) {
	game, err := s.store.GetGame(ctx, guildID, gameID)
	if err != nil {
		return GameStats{}, err
	}
	return s.gameStats(ctx, guildID, game)
}

func (s *Service) gameStats(ctx context.Context, guildID string, game store.Game) (GameStats, error) {
	cards, err := s.store.ListCards(ctx, guildID, game.ID)
	if err != nil {
		return GameStats{}, err
	}
	stats := GameStats{Game: game, PlayerCount: len(cards)}
	for _, c := range cards {
		marks := c.Marks()
		marked := 0
		for i, m := range marks {
			if m && i != bingo.CenterIdx {
				marked++
			}
		}
		stats.Leaders = append(stats.Leaders, PlayerProgress{
			UserID:   c.UserID,
			CardID:   c.ID,
			Marked:   marked,
			BestLine: bingo.BestProgress(marks),
			HasBingo: bingo.HasBingo(marks),
		})
	}
	sort.SliceStable(stats.Leaders, func(i, j int) bool {
		a, b := stats.Leaders[i], stats.Leaders[j]
		if a.BestLine != b.BestLine {
			return a.BestLine > b.BestLine
		}
		return a.Marked > b.Marked
	})
	return stats, nil
}
