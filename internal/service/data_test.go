package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Syndic0r/gw2-raid-bingo/internal/store"
)

// TestDataMutationsRequireAdmin verifies every admin-gated data mutation in the
// service layer rejects a non-admin with ErrForbidden and lets an admin through.
// This is the guarantee that a member of one guild (or an outsider) cannot alter
// another guild's square library through any entry point.
func TestDataMutationsRequireAdmin(t *testing.T) {
	svc, st := newSvc(t, "admin")
	sharedID := seed(t, st)
	ctx := context.Background()

	// Grab a real entry id in the seeded shared pool to exercise edit/remove.
	entries, err := st.ListEntries(ctx, guild, sharedID, true)
	if err != nil || len(entries) == 0 {
		t.Fatalf("seed entries: %v (n=%d)", err, len(entries))
	}
	entryID := entries[0].ID

	// Each mutation, run first as a non-admin (want ErrForbidden) then as admin.
	cases := []struct {
		name string
		call func(userID string) error
	}{
		{"CreateSharedPool", func(u string) error {
			_, err := svc.CreateSharedPool(ctx, guild, u, "pool-"+u, "Pool")
			return err
		}},
		{"AddEntry", func(u string) error {
			_, err := svc.AddEntry(ctx, guild, u, sharedID, "new square")
			return err
		}},
		{"EditEntry", func(u string) error {
			return svc.EditEntry(ctx, guild, u, entryID, "edited")
		}},
		{"ClearPool", func(u string) error {
			_, err := svc.ClearPool(ctx, guild, u, sharedID)
			return err
		}},
		{"DeleteSharedPool", func(u string) error {
			return svc.DeleteSharedPool(ctx, guild, u, sharedID)
		}},
	}

	for _, tc := range cases {
		if err := tc.call("rando"); !errors.Is(err, ErrForbidden) {
			t.Errorf("%s as non-admin: got %v, want ErrForbidden", tc.name, err)
		}
	}
	// As admin the same calls must not be rejected for authorization reasons.
	for _, tc := range cases {
		if err := tc.call("admin"); errors.Is(err, ErrForbidden) {
			t.Errorf("%s as admin: unexpectedly forbidden", tc.name)
		}
	}
}

// TestCrossGuildEntryEditIsScoped confirms that even an admin cannot edit an
// entry belonging to a DIFFERENT guild: the store scopes every write by guild id,
// so the update targets no row and the original entry is left untouched. This is
// the store-level backstop behind the service admin gate.
func TestCrossGuildEntryEditIsScoped(t *testing.T) {
	svc, st := newSvc(t, "admin")
	sharedID := seed(t, st)
	ctx := context.Background()

	entries, err := st.ListEntries(ctx, guild, sharedID, true)
	if err != nil || len(entries) == 0 {
		t.Fatalf("seed entries: %v", err)
	}
	entry := entries[0]

	// Acting against a different guild id, the guild-scoped WHERE matches no row.
	const otherGuild = "g2"
	if err := st.EnsureGuild(ctx, otherGuild); err != nil {
		t.Fatal(err)
	}
	if err := svc.EditEntry(ctx, otherGuild, "admin", entry.ID, "hijacked"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-guild edit: got %v, want ErrNotFound", err)
	}
	// The original entry in guild g1 must be unchanged.
	after, err := st.ListEntries(ctx, guild, sharedID, true)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Text != entry.Text {
		t.Errorf("entry text changed across guilds: %q -> %q", entry.Text, after[0].Text)
	}
}
