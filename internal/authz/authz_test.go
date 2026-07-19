package authz

import "testing"

func TestIsBingoAdmin(t *testing.T) {
	cfg := GuildConfig{AdminRoleIDs: []string{"role-raid-lead", "role-officer"}}
	cases := []struct {
		name string
		m    Member
		want bool
	}{
		{"guild owner", Member{IsGuildOwner: true}, true},
		{"has administrator role", Member{RoleIDs: []string{"r1"}, AdministratorRoleIDs: []string{"r1"}}, true},
		{"holds a configured admin role", Member{RoleIDs: []string{"role-officer"}}, true},
		{"holds one of several configured roles", Member{RoleIDs: []string{"x", "role-raid-lead"}}, true},
		{"plain member", Member{RoleIDs: []string{"role-member"}}, false},
		{"no roles at all", Member{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBingoAdmin(tc.m, cfg); got != tc.want {
				t.Errorf("IsBingoAdmin = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsBingoAdmin_NoConfiguredRoles(t *testing.T) {
	// With no configured admin roles, only owner/administrator qualify.
	empty := GuildConfig{}
	if IsBingoAdmin(Member{RoleIDs: []string{"anything"}}, empty) {
		t.Error("plain member must not be admin when no admin roles are configured")
	}
	if !IsBingoAdmin(Member{IsGuildOwner: true}, empty) {
		t.Error("owner must always be admin")
	}
}

func TestRequireDiscordAdministrator(t *testing.T) {
	// A configured bingo-admin role does NOT grant access to /setup.
	m := Member{RoleIDs: []string{"role-officer"}}
	if RequireDiscordAdministrator(m) {
		t.Error("a configured bingo-admin role must not unlock /setup")
	}
	if !RequireDiscordAdministrator(Member{IsGuildOwner: true}) {
		t.Error("owner must pass /setup gate")
	}
	if !RequireDiscordAdministrator(Member{AdministratorRoleIDs: []string{"r1"}}) {
		t.Error("administrator-permission role must pass /setup gate")
	}
}
