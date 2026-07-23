package service

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/Syndic0r/gw2-raid-bingo/internal/authz"
	"github.com/Syndic0r/gw2-raid-bingo/internal/events"
	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

const guild = "g1"

// fakeResolver marks specific users as guild admins.
type fakeResolver struct{ admins map[string]bool }

func (f fakeResolver) Resolve(_ context.Context, _, userID string) (authz.Member, error) {
	if f.admins[userID] {
		return authz.Member{IsGuildOwner: true}, nil
	}
	return authz.Member{RoleIDs: []string{"member"}}, nil
}

func newSvc(t *testing.T, admins ...string) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	set := map[string]bool{}
	for _, a := range admins {
		set[a] = true
	}
	svc := New(st, events.NewHub(), fakeResolver{admins: set})
	svc.rng = func() *rand.Rand { return rand.New(rand.NewSource(1)) }
	return svc, st
}

func seed(t *testing.T, st *store.Store) int64 {
	t.Helper()
	ctx := context.Background()
	if err := st.EnsureGuild(ctx, guild); err != nil {
		t.Fatal(err)
	}
	// A game cannot be opened until an announcement channel is configured.
	if err := st.SetAnnounceChannel(ctx, guild, "announce-chan"); err != nil {
		t.Fatal(err)
	}
	shared, _ := st.CreatePool(ctx, guild, "general", "General")
	for i := 0; i < 30; i++ {
		st.AddEntry(ctx, guild, shared.ID, "shared "+string(rune('a'+i)))
	}
	return shared.ID
}

func TestNewGameRequiresAdmin(t *testing.T) {
	svc, st := newSvc(t, "admin")
	sharedID := seed(t, st)
	ctx := context.Background()

	if _, err := svc.NewGame(ctx, guild, "rando", "", []int64{sharedID}, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin NewGame: got %v, want ErrForbidden", err)
	}
	if _, err := svc.NewGame(ctx, guild, "admin", "", []int64{sharedID}, false); err != nil {
		t.Fatalf("admin NewGame: %v", err)
	}
}

func TestNewGameRequiresAnnounceChannel(t *testing.T) {
	svc, st := newSvc(t, "admin")
	ctx := context.Background()
	// Set up a guild with data but NO announcement channel.
	if err := st.EnsureGuild(ctx, guild); err != nil {
		t.Fatal(err)
	}
	shared, _ := st.CreatePool(ctx, guild, "general", "General")
	for i := 0; i < 30; i++ {
		st.AddEntry(ctx, guild, shared.ID, "shared "+string(rune('a'+i)))
	}
	if _, err := svc.NewGame(ctx, guild, "admin", "", []int64{shared.ID}, false); !errors.Is(err, ErrNoAnnounceChannel) {
		t.Fatalf("got %v, want ErrNoAnnounceChannel", err)
	}
	// Once a channel is set, it works.
	if err := st.SetAnnounceChannel(ctx, guild, "chan"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.NewGame(ctx, guild, "admin", "", []int64{shared.ID}, false); err != nil {
		t.Fatalf("after channel set: %v", err)
	}
}

func TestDealAndPlayThroughService(t *testing.T) {
	svc, st := newSvc(t, "admin")
	sharedID := seed(t, st)
	ctx := context.Background()

	game, err := svc.NewGame(ctx, guild, "admin", "", []int64{sharedID}, false)
	if err != nil {
		t.Fatal(err)
	}

	// A plain member can deal a card.
	card, _, err := svc.DealCard(ctx, guild, "player1", game.ID)
	if err != nil {
		t.Fatal(err)
	}

	// A different member cannot toggle player1's card.
	if _, _, err := svc.ToggleCell(ctx, guild, "player2", card.ID, 0); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign toggle: got %v, want ErrForbidden", err)
	}
	// The owner can.
	if _, _, err := svc.ToggleCell(ctx, guild, "player1", card.ID, 0); err != nil {
		t.Fatalf("owner toggle: %v", err)
	}
	// An admin can toggle anyone's card (used for corrections/inspection tools).
	if _, _, err := svc.ToggleCell(ctx, guild, "admin", card.ID, 1); err != nil {
		t.Fatalf("admin toggle: %v", err)
	}

	// Complete the top row and call bingo.
	for _, idx := range []int{0, 1, 2, 3, 4} {
		if _, _, err := svc.ToggleCell(ctx, guild, "player1", card.ID, idx); err != nil {
			// idx 0 and 1 may already be on from above; toggling twice is fine as
			// long as we end marked. Re-mark any that got turned off.
		}
	}
	// Ensure the whole row is marked (some toggles above may have flipped cells off).
	current, _ := st.GetCard(ctx, guild, card.ID)
	for _, idx := range []int{0, 1, 2, 3, 4} {
		if !current.Cells[idx].Marked {
			svc.ToggleCell(ctx, guild, "player1", card.ID, idx)
		}
	}

	res, err := svc.CallBingo(ctx, guild, "player1", card.ID)
	if err != nil {
		t.Fatalf("call bingo: %v", err)
	}
	if res.Game.WinnerUserID != "player1" {
		t.Fatalf("winner = %q, want player1", res.Game.WinnerUserID)
	}
}

func TestImportRequiresAdminAndCounts(t *testing.T) {
	svc, st := newSvc(t, "admin")
	if err := st.EnsureGuild(context.Background(), guild); err != nil {
		t.Fatal(err)
	}
	// "w2" already exists as a default wing pool; "memes" is created on import.
	data := store.SeedData{
		Instance: map[string][]string{"w2": {"a", "b"}},
		Shared:   map[string][]string{"memes": {"x", "y", "z"}},
	}
	if _, err := svc.ImportData(context.Background(), guild, "rando", data); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-admin import: got %v, want ErrForbidden", err)
	}
	res, err := svc.ImportData(context.Background(), guild, "admin", data)
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted != 5 || res.PoolsMade != 1 {
		t.Fatalf("import result = %+v, want 5 inserted, 1 pool", res)
	}
}

func TestGameStats(t *testing.T) {
	svc, st := newSvc(t, "admin")
	sharedID := seed(t, st)
	ctx := context.Background()
	game, _ := svc.NewGame(ctx, guild, "admin", "", []int64{sharedID}, false)
	svc.DealCard(ctx, guild, "p1", game.ID)
	svc.DealCard(ctx, guild, "p2", game.ID)

	stats, err := svc.GameStatsForGame(ctx, guild, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.PlayerCount != 2 {
		t.Fatalf("player count = %d, want 2", stats.PlayerCount)
	}
	// Every fresh card has the free centre, so best line is at least 1.
	for _, p := range stats.Leaders {
		if p.BestLine < 1 {
			t.Errorf("best line %d < 1", p.BestLine)
		}
	}
}
