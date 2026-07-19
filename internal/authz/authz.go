// Package authz holds the single "is this member a bingo admin" rule, kept as a
// pure function so the Discord command handlers, the button handlers, and the
// web API all decide authorization identically. Fetching the inputs (a member's
// roles, which roles carry Discord's Administrator permission) belongs to the
// Discord layer; this package only decides.
package authz

// Discord's Administrator permission bit. A role with this bit set can do
// anything in the guild, so its holders are always treated as bingo admins.
const PermAdministrator int64 = 0x00000008

// Member describes the caller as seen from Discord, reduced to what the admin
// rule needs.
type Member struct {
	// IsGuildOwner is true for the single guild owner.
	IsGuildOwner bool
	// RoleIDs are the role IDs the member currently holds.
	RoleIDs []string
	// AdministratorRoleIDs are the member's roles that carry the Administrator
	// permission (resolved by the caller from the guild's role list). It is a
	// subset of RoleIDs.
	AdministratorRoleIDs []string
}

// GuildConfig is the per-guild bingo configuration relevant to authorization.
type GuildConfig struct {
	// AdminRoleIDs are the roles an admin explicitly granted bingo-admin rights
	// via /setup. May be empty.
	AdminRoleIDs []string
}

// IsBingoAdmin reports whether the member may run admin-only actions (open or
// abort a game, edit data, toggle other players' cells). The member qualifies if
// any of the following holds:
//
//   - they are the guild owner, or
//   - they hold any role with Discord's Administrator permission, or
//   - they hold any of the guild's configured bingo-admin roles.
func IsBingoAdmin(m Member, cfg GuildConfig) bool {
	if m.IsGuildOwner {
		return true
	}
	if len(m.AdministratorRoleIDs) > 0 {
		return true
	}
	return intersects(m.RoleIDs, cfg.AdminRoleIDs)
}

// RequireDiscordAdministrator reports whether the member may run /setup itself,
// which is gated on Discord's own Administrator permission (owner included) and
// deliberately not on the configured bingo-admin roles - only a real server
// admin should be able to choose who the bingo admins are.
func RequireDiscordAdministrator(m Member) bool {
	return m.IsGuildOwner || len(m.AdministratorRoleIDs) > 0
}

func intersects(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, x := range a {
		set[x] = struct{}{}
	}
	for _, y := range b {
		if _, ok := set[y]; ok {
			return true
		}
	}
	return false
}
