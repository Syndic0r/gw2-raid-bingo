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

// fillPool adds n distinct entries to a pool.
func fillPool(t *testing.T, s *Store, poolID int64, n int, prefix string) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		if _, err := s.AddEntry(ctx, guild, poolID, prefix+" "+string(rune('a'+i%26))+string(rune('a'+i/26))); err != nil {
			t.Fatal(err)
		}
	}
}

// seedGuild sets up a guild with two pools that each hold enough entries to fill a
// card, so games can be opened. Returns the two pool ids.
func seedGuild(t *testing.T, s *Store) (poolA, poolB int64) {
	t.Helper()
	ctx := context.Background()
	if err := s.EnsureGuild(ctx, guild); err != nil {
		t.Fatal(err)
	}
	// EnsureGuild created blank wing pools; reuse w1 as poolA and fill it.
	a, err := s.GetPool(ctx, guild, "w1")
	if err != nil {
		t.Fatal(err)
	}
	fillPool(t, s, a.ID, bingo.FillCount, "wing")
	b, err := s.CreatePool(ctx, guild, "general", "General")
	if err != nil {
		t.Fatal(err)
	}
	fillPool(t, s, b.ID, 30, "general")
	return a.ID, b.ID
}

func TestEnsureGuild_CreatesDefaultPools(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.EnsureGuild(ctx, guild); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureGuild(ctx, guild); err != nil { // idempotent
		t.Fatal(err)
	}
	pools, err := s.ListPools(ctx, guild)
	if err != nil {
		t.Fatal(err)
	}
	if len(pools) != len(DefaultPools()) {
		t.Fatalf("got %d default pools, want %d", len(pools), len(DefaultPools()))
	}
	// A deleted default pool is NOT recreated on the next EnsureGuild.
	if err := s.DeletePool(ctx, guild, pools[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureGuild(ctx, guild); err != nil {
		t.Fatal(err)
	}
	pools, _ = s.ListPools(ctx, guild)
	if len(pools) != len(DefaultPools())-1 {
		t.Fatalf("deleted default pool was recreated: got %d pools", len(pools))
	}
}

func TestPoolSetKeyCanonical(t *testing.T) {
	// Order-independent and dedup-stable: same set -> same key.
	if poolSetKey([]int64{3, 1, 2}) != poolSetKey([]int64{2, 3, 1, 1}) {
		t.Fatal("same set in different order/dupes should produce the same key")
	}
	if poolSetKey([]int64{1, 2}) == poolSetKey([]int64{1, 2, 3}) {
		t.Fatal("different sets must produce different keys")
	}
	if poolSetKey([]int64{5, 2, 9}) != "2,5,9" {
		t.Fatalf("unexpected key %q", poolSetKey([]int64{5, 2, 9}))
	}
}

func TestNewGame_Validations(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	poolA, _ := seedGuild(t, s)

	// Empty selection is rejected.
	if _, err := s.NewGame(ctx, guild, "", "u", nil, false); !errors.Is(err, ErrNoPoolsSelected) {
		t.Fatalf("empty set: got %v, want ErrNoPoolsSelected", err)
	}
	// A pool with too few entries is rejected at creation (not deferred to deal).
	thin, _ := s.CreatePool(ctx, guild, "thin", "Thin")
	fillPool(t, s, thin.ID, 5, "thin")
	if _, err := s.NewGame(ctx, guild, "", "u", []int64{thin.ID}, false); !errors.Is(err, ErrValidation) {
		t.Fatalf("too-few entries: got %v, want ErrValidation", err)
	}
	// A pool id from another guild is rejected.
	if _, err := s.NewGame(ctx, guild, "", "u", []int64{999999}, false); !errors.Is(err, ErrValidation) {
		t.Fatalf("cross-guild pool: got %v, want ErrValidation", err)
	}
	// A valid single pool opens, and its name is derived from the pool names.
	g, err := s.NewGame(ctx, guild, "", "u", []int64{poolA}, false)
	if err != nil {
		t.Fatalf("valid game: %v", err)
	}
	if g.Name != "Wing 1" {
		t.Fatalf("derived name = %q, want %q", g.Name, "Wing 1")
	}
	// A custom name overrides the derived one.
	g2, err := s.NewGame(ctx, guild, "Friday raid", "u", []int64{poolA, thin.ID}, false)
	if err != nil {
		t.Fatal(err)
	}
	if g2.Name != "Friday raid" {
		t.Fatalf("custom name = %q", g2.Name)
	}
}

func TestNewGame_OneOpenPerPoolSet(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	poolA, poolB := seedGuild(t, s)

	if _, err := s.NewGame(ctx, guild, "", "u", []int64{poolA}, false); err != nil {
		t.Fatal(err)
	}
	// Same set (order/dupes differ) while open -> rejected.
	if _, err := s.NewGame(ctx, guild, "", "u", []int64{poolA, poolA}, false); !errors.Is(err, ErrGameOpen) {
		t.Fatalf("same set: got %v, want ErrGameOpen", err)
	}
	// A different set opens independently.
	if _, err := s.NewGame(ctx, guild, "", "u", []int64{poolA, poolB}, false); err != nil {
		t.Fatalf("different set should open: %v", err)
	}
	// Replace aborts the open game with the same set and opens a fresh one.
	g, err := s.NewGame(ctx, guild, "", "u", []int64{poolA}, true)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if g.Status != StatusOpen {
		t.Fatalf("replacement status %q", g.Status)
	}
	open, err := s.ListOpenGames(ctx, guild)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 { // {poolA} (replaced) + {poolA,poolB}
		t.Fatalf("open games = %d, want 2", len(open))
	}
}

func TestDeletePool_CascadesAndAllowed(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	poolA, _ := seedGuild(t, s)
	// A former wing pool is an ordinary, deletable pool now.
	if err := s.DeletePool(ctx, guild, poolA); err != nil {
		t.Fatalf("delete pool: %v", err)
	}
	if _, err := s.GetPool(ctx, guild, "w1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pool still present: %v", err)
	}
	// Its entries cascaded away.
	entries, _ := s.ListEntries(ctx, guild, poolA, false)
	if len(entries) != 0 {
		t.Fatalf("entries survived pool delete: %d", len(entries))
	}
	// Deleting a nonexistent pool is ErrNotFound.
	if err := s.DeletePool(ctx, guild, poolA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("re-delete: got %v, want ErrNotFound", err)
	}
}

func TestDealDegradesWhenPoolDeleted(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	poolA, poolB := seedGuild(t, s)
	// Open a game over both pools, then delete one so the union drops below a card.
	game, err := s.NewGame(ctx, guild, "", "u", []int64{poolA}, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = poolB
	if err := s.DeletePool(ctx, guild, poolA); err != nil {
		t.Fatal(err)
	}
	// A subsequent deal degrades gracefully instead of panicking.
	if _, err := s.GetOrDealCard(ctx, guild, game.ID, "u1", rand.New(rand.NewSource(1))); !errors.Is(err, bingo.ErrNotEnoughEntries) {
		t.Fatalf("deal after pool delete: got %v, want ErrNotEnoughEntries", err)
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
	poolA, _ := seedGuild(t, s)
	if _, err := s.AddEntry(ctx, guild, poolA, "   "); !errors.Is(err, ErrValidation) {
		t.Errorf("empty text: got %v, want ErrValidation", err)
	}
	long := make([]byte, MaxEntryTextLen+1)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := s.AddEntry(ctx, guild, poolA, string(long)); !errors.Is(err, ErrValidation) {
		t.Errorf("too-long text: got %v, want ErrValidation", err)
	}
}

func TestFullGame_DealMarkCallBingo(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, sharedID := seedGuild(t, s)

	game, err := s.NewGame(ctx, guild, "", "u-host", []int64{sharedID}, false)
	if err != nil {
		t.Fatal(err)
	}

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

	if _, err := s.CallBingo(ctx, guild, c1.ID, "u1"); !errors.Is(err, ErrNoBingo) {
		t.Fatalf("premature call: got %v, want ErrNoBingo", err)
	}

	for _, idx := range []int{0, 1, 2, 3, 4} {
		if _, _, err := s.ToggleCell(ctx, guild, c1.ID, idx, true); err != nil {
			t.Fatalf("toggle %d: %v", idx, err)
		}
	}
	_, hasBingo, err := s.ToggleCell(ctx, guild, c1.ID, 0, true) // off
	if err != nil {
		t.Fatal(err)
	}
	if hasBingo {
		t.Fatal("row should be incomplete after un-marking a cell")
	}
	_, hasBingo, err = s.ToggleCell(ctx, guild, c1.ID, 0, true) // back on
	if err != nil {
		t.Fatal(err)
	}
	if !hasBingo {
		t.Fatal("row should be complete")
	}

	if _, _, err := s.ToggleCell(ctx, guild, c1.ID, 6, false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("got %v, want ErrForbidden", err)
	}

	res, err := s.CallBingo(ctx, guild, c1.ID, "u1")
	if err != nil {
		t.Fatalf("call bingo: %v", err)
	}
	if res.Game.Status != StatusFinished || res.Game.WinnerUserID != "u1" || res.Game.WinnerCardID != c1.ID {
		t.Fatalf("unexpected finished game: %+v", res.Game)
	}

	if _, _, err := s.ToggleCell(ctx, guild, c1.ID, 7, true); !errors.Is(err, ErrGameNotOpen) {
		t.Fatalf("toggle after finish: got %v, want ErrGameNotOpen", err)
	}
	if _, err := s.CallBingo(ctx, guild, c1.ID, "u1"); !errors.Is(err, ErrGameNotOpen) {
		t.Fatalf("second call: got %v, want ErrGameNotOpen", err)
	}

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
	game, _ := s.NewGame(ctx, guild, "", "u-host", []int64{sharedID}, false)
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
	poolA, _ := seedGuild(t, s)
	e, err := s.AddEntry(ctx, guild, poolA, "temporary square")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SoftDeleteEntry(ctx, guild, e.ID); err != nil {
		t.Fatal(err)
	}
	active, _ := s.ListEntries(ctx, guild, poolA, true)
	for _, a := range active {
		if a.ID == e.ID {
			t.Fatal("soft-deleted entry still active")
		}
	}
	all, _ := s.ListEntries(ctx, guild, poolA, false)
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
	poolA, _ := seedGuild(t, s) // poolA has FillCount entries
	n, err := s.ClearPoolEntries(ctx, guild, poolA)
	if err != nil {
		t.Fatal(err)
	}
	if int(n) != bingo.FillCount {
		t.Fatalf("cleared %d, want %d", n, bingo.FillCount)
	}
	active, _ := s.ListEntries(ctx, guild, poolA, true)
	if len(active) != 0 {
		t.Fatalf("pool still has %d active entries after clear", len(active))
	}
	if _, err := s.ClearPoolEntries(ctx, "other-guild", poolA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-guild clear: got %v, want ErrNotFound", err)
	}
}

func TestUnicodeEntriesRoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.EnsureGuild(ctx, guild); err != nil {
		t.Fatal(err)
	}
	p, _ := s.CreatePool(ctx, guild, "intl", "Intl")
	texts := []string{"漢字ビンゴ", "بينغو", "Ёлки-палки", "emoji 🎉🎲", "combining é"}
	for _, txt := range texts {
		e, err := s.AddEntry(ctx, guild, p.ID, txt)
		if err != nil {
			t.Fatalf("add %q: %v", txt, err)
		}
		if e.Text != txt {
			t.Fatalf("stored %q, want %q", e.Text, txt)
		}
	}
	got, _ := s.ListEntries(ctx, guild, p.ID, true)
	if len(got) != len(texts) {
		t.Fatalf("got %d entries, want %d", len(got), len(texts))
	}
}
