package discord

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Syndic0r/gw2-raid-bingo/internal/authz"
)

// resolver implements service.RoleResolver over the Discord REST API, resolving
// a member's owner flag, roles, and which roles carry the Administrator
// permission. Both the guild role list and each member's standing are cached for
// a few minutes: roles change rarely, and every web request and Discord command
// resolves permissions, so hitting Discord's API each time made every click wait
// on a REST round-trip. The cache is refreshed lazily on the first access after
// it expires (one call every few minutes per active user), and /setup clears it.
type resolver struct {
	session *discordgo.Session

	mu      sync.Mutex
	cache   map[string]cachedGuild
	members map[string]cachedMember
}

type cachedGuild struct {
	ownerID       string
	adminRoleIDs  map[string]struct{} // roles with the Administrator permission
	expiresAtUnix int64
}

type cachedMember struct {
	member        authz.Member
	expiresAtUnix int64
}

func newResolver(s *discordgo.Session) *resolver {
	return &resolver{
		session: s,
		cache:   make(map[string]cachedGuild),
		members: make(map[string]cachedMember),
	}
}

const (
	guildCacheTTL  = 5 * time.Minute
	memberCacheTTL = 5 * time.Minute
)

// Resolve returns the member's authz standing in a guild, from cache when fresh.
func (r *resolver) Resolve(ctx context.Context, guildID, userID string) (authz.Member, error) {
	key := guildID + ":" + userID
	now := time.Now().Unix()

	r.mu.Lock()
	if c, ok := r.members[key]; ok && c.expiresAtUnix > now {
		r.mu.Unlock()
		return c.member, nil
	}
	r.mu.Unlock()

	g, err := r.guildInfo(guildID)
	if err != nil {
		return authz.Member{}, err
	}
	member, err := r.session.GuildMember(guildID, userID, discordgo.WithContext(ctx))
	if err != nil {
		return authz.Member{}, err
	}
	m := authz.Member{
		IsGuildOwner: userID == g.ownerID,
		RoleIDs:      member.Roles,
	}
	for _, roleID := range member.Roles {
		if _, ok := g.adminRoleIDs[roleID]; ok {
			m.AdministratorRoleIDs = append(m.AdministratorRoleIDs, roleID)
		}
	}

	r.mu.Lock()
	r.members[key] = cachedMember{member: m, expiresAtUnix: now + int64(memberCacheTTL.Seconds())}
	// The cache is filled lazily and only read entries are TTL-checked, so expired
	// entries for users who never come back would pile up forever. Sweep them once
	// the map grows past a comfortable bound (cheap: runs rarely, under the lock).
	if len(r.members) > 4096 {
		for k, v := range r.members {
			if v.expiresAtUnix <= now {
				delete(r.members, k)
			}
		}
	}
	r.mu.Unlock()
	return m, nil
}

func (r *resolver) guildInfo(guildID string) (cachedGuild, error) {
	now := time.Now().Unix()
	r.mu.Lock()
	if c, ok := r.cache[guildID]; ok && c.expiresAtUnix > now {
		r.mu.Unlock()
		return c, nil
	}
	r.mu.Unlock()

	guild, err := r.session.Guild(guildID)
	if err != nil {
		return cachedGuild{}, err
	}
	admins := make(map[string]struct{})
	for _, role := range guild.Roles {
		if role.Permissions&discordgo.PermissionAdministrator != 0 {
			admins[role.ID] = struct{}{}
		}
	}
	c := cachedGuild{ownerID: guild.OwnerID, adminRoleIDs: admins, expiresAtUnix: now + int64(guildCacheTTL.Seconds())}
	r.mu.Lock()
	r.cache[guildID] = c
	r.mu.Unlock()
	return c, nil
}

// invalidate drops a guild's cached role info and all its cached members (called
// after /setup changes), so admin-role changes take effect immediately.
func (r *resolver) invalidate(guildID string) {
	r.mu.Lock()
	delete(r.cache, guildID)
	prefix := guildID + ":"
	for key := range r.members {
		if strings.HasPrefix(key, prefix) {
			delete(r.members, key)
		}
	}
	r.mu.Unlock()
}
