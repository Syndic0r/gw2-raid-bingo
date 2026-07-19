package store

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/Syndic0r/gw2-raid-bingo/internal/bingo"
)

const guild = "guild-1"

func newStore(t *testing.T) *Store {
	t.Helper()
	// A file-less shared-cache memory DB keeps the single connection's schema.
	s, err := Open(context.Background(), "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedGuild sets up a guild with a full set of entries so cards can be dealt.
func seedGuild(t *testing.T, s *Store) (instPoolID, sharedPoolID int64) {
	t.Helper()
	ctx := context.Background()
	if err := s.EnsureGuild(ctx, guild); err != nil {
		t.Fatal(err)
	}
	inst, err := s.InstancePool(ctx, guild, bingo.W1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddEntry(ctx, guild, inst.ID, "vale guardian teleports people"); err != nil {
		t.Fatal(err)
	}
	shared, err := s.CreateSharedPool(ctx, guild, "general", "General")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		if _, err := s.AddEntry(ctx, guild, shared.ID, "general line "+string(rune('a'+i))); err != nil {
			t.Fatal(err)
		}
	}
	return inst.ID, shared.ID
}

func TestEnsureGuild_CreatesInstancePools(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.EnsureGuild(ctx, guild); err != nil {
		t.Fatal(err)
	}
	// Idempotent.
	if err := s.EnsureGuild(ctx, guild); err != nil {
		t.Fatal(err)
	}
	pools, err := s.ListPools(ctx, guild, KindInstance)
	if err != nil {
		t.Fatal(err)
	}
	if len(pools) != len(bingo.Instances()) {
		t.Fatalf("got %d instance pools, want %d", len(pools), len(bingo.Instances()))
	}
}

func TestAdminRolesRoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.EnsureGuild(ctx, guild); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAdminRoles(ctx, guild, []string{"r1", "r2", "r2", ""}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetAdminRoles(ctx, guild)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 deduped roles", got)
	}
	// Replace semantics.
	if err := s.SetAdminRoles(ctx, guild, []string{"r3"}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetAdminRoles(ctx, guild)
	if len(got) != 1 || got[0] != "r3" {
		t.Fatalf("got %v, want [r3]", got)
	}
}

func TestEntryValidation(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	instPoolID, _ := seedGuild(t, s)
	if _, err := s.AddEntry(ctx, guild, instPoolID, "   "); !errors.Is(err, ErrValidation) {
		t.Errorf("empty text: got %v, want ErrValidation", err)
	}
	long := make([]byte, MaxEntryTextLen+1)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := s.AddEntry(ctx, guild, instPoolID, string(long)); !errors.Is(err, ErrValidation) {
		t.Errorf("too-long text: got %v, want ErrValidation", err)
	}
}

func TestNotEnoughEntriesToDeal(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.EnsureGuild(ctx, guild); err != nil {
		t.Fatal(err)
	}
	inst, _ := s.InstancePool(ctx, guild, bingo.W1)
	if _, err := s.AddEntry(ctx, guild, inst.ID, "only one"); err != nil {
		t.Fatal(err)
	}
	game, err := s.NewGame(ctx, guild, bingo.W1, "u-host", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.GetOrDealCard(ctx, guild, game.ID, "u1", nil)
	if !errors.Is(err, bingo.ErrNotEnoughEntries) {
		t.Fatalf("got %v, want ErrNotEnoughEntries", err)
	}
}

func TestNewGame_OneOpenPerInstance(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	seedGuild(t, s)
	if _, err := s.NewGame(ctx, guild, bingo.W1, "u-host", nil, false); err != nil {
		t.Fatal(err)
	}
	// Second open without replace is rejected.
	if _, err := s.NewGame(ctx, guild, bingo.W1, "u-host", nil, false); !errors.Is(err, ErrGameOpen) {
		t.Fatalf("got %v, want ErrGameOpen", err)
	}
	// A different instance is independent.
	if _, err := s.NewGame(ctx, guild, bingo.W2, "u-host", nil, false); err != nil {
		t.Fatalf("second instance should open: %v", err)
	}
	// Replace aborts the old game and opens a new one.
	g2, err := s.NewGame(ctx, guild, bingo.W1, "u-host", nil, true)
	if err != nil {
		t.Fatalf("replace should succeed: %v", err)
	}
	if g2.Status != StatusOpen {
		t.Fatalf("replacement game status %q", g2.Status)
	}
}

func TestFullGame_DealMarkCallBingo(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, sharedID := seedGuild(t, s)

	game, err := s.NewGame(ctx, guild, bingo.W1, "u-host", []int64{sharedID}, false)
	if err != nil {
		t.Fatal(err)
	}

	// Two players deal in; dealing is idempotent per user.
	c1, err := s.GetOrDealCard(ctx, guild, game.ID, "u1", rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.GetOrDealCard(ctx, guild, game.ID, "u1", rand.New(rand.NewSource(999)))
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != c1.ID {
		t.Fatal("re-dealing gave a different card")
	}
	if _, err := s.GetOrDealCard(ctx, guild, game.ID, "u2", rand.New(rand.NewSource(2))); err != nil {
		t.Fatal(err)
	}

	// Calling bingo without a line is refused.
	if _, err := s.CallBingo(ctx, guild, c1.ID, "u1"); !errors.Is(err, ErrNoBingo) {
		t.Fatalf("premature call: got %v, want ErrNoBingo", err)
	}

	// Mark the top row (cells 0..4) on u1's card to complete a line.
	for _, idx := range []int{0, 1, 2, 3, 4} {
		_, _, err := s.ToggleCell(ctx, guild, c1.ID, idx, true)
		if err != nil {
			t.Fatalf("toggle %d: %v", idx, err)
		}
	}
	card, hasBingo, err := s.ToggleCell(ctx, guild, c1.ID, 0, true) // toggle 0 off...
	if err != nil {
		t.Fatal(err)
	}
	if hasBingo {
		t.Fatal("row should be incomplete after un-marking a cell")
	}
	// ...and back on.
	card, hasBingo, err = s.ToggleCell(ctx, guild, c1.ID, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if !hasBingo {
		t.Fatal("row should be complete")
	}
	_ = card

	// A non-owner/non-admin cannot toggle.
	if _, _, err := s.ToggleCell(ctx, guild, c1.ID, 6, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}

	// Call bingo.
	res, err := s.CallBingo(ctx, guild, c1.ID, "u1")
	if err != nil {
		t.Fatalf("call bingo: %v", err)
	}
	if res.Game.Status != StatusFinished || res.Game.WinnerUserID != "u1" || res.Game.WinnerCardID != c1.ID {
		t.Fatalf("unexpected finished game: %+v", res.Game)
	}

	// Game is now closed: no more toggles, and a second call loses the race.
	if _, _, err := s.ToggleCell(ctx, guild, c1.ID, 7, true); !errors.Is(err, ErrGameNotOpen) {
		t.Fatalf("toggle after finish: got %v, want ErrGameNotOpen", err)
	}
	if _, err := s.CallBingo(ctx, guild, c1.ID, "u1"); !errors.Is(err, ErrGameNotOpen) {
		t.Fatalf("second call: got %v, want ErrGameNotOpen", err)
	}

	// The finished game and its cards remain readable as history.
	cards, err := s.ListCards(ctx, guild, game.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 {
		t.Fatalf("history has %d cards, want 2", len(cards))
	}
}

func TestToggleFreeCentreRejected(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, sharedID := seedGuild(t, s)
	game, _ := s.NewGame(ctx, guild, bingo.W1, "u-host", []int64{sharedID}, false)
	c, err := s.GetOrDealCard(ctx, guild, game.ID, "u1", rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ToggleCell(ctx, guild, c.ID, bingo.CenterIdx, true); !errors.Is(err, ErrCellFree) {
		t.Fatalf("got %v, want ErrCellFree", err)
	}
}

func TestSoftDeleteEntryKeepsHistory(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	instPoolID, _ := seedGuild(t, s)
	e, err := s.AddEntry(ctx, guild, instPoolID, "temporary square")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SoftDeleteEntry(ctx, guild, e.ID); err != nil {
		t.Fatal(err)
	}
	active, err := s.ListEntries(ctx, guild, instPoolID, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range active {
		if a.ID == e.ID {
			t.Fatal("soft-deleted entry still active")
		}
	}
	all, err := s.ListEntries(ctx, guild, instPoolID, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range all {
		if a.ID == e.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("soft-deleted entry missing from full listing")
	}
}

func TestClearPoolEntries(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	instPoolID, _ := seedGuild(t, s)
	// seedGuild added 1 instance entry; add two more.
	s.AddEntry(ctx, guild, instPoolID, "square two")
	s.AddEntry(ctx, guild, instPoolID, "square three")
	n, err := s.ClearPoolEntries(ctx, guild, instPoolID)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("cleared %d, want 3", n)
	}
	active, _ := s.ListEntries(ctx, guild, instPoolID, true)
	if len(active) != 0 {
		t.Fatalf("pool still has %d active entries after clear", len(active))
	}
	// A wrong-guild clear is ErrNotFound.
	if _, err := s.ClearPoolEntries(ctx, "other-guild", instPoolID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-guild clear: got %v, want ErrNotFound", err)
	}
}
