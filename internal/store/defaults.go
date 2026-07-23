package store

// DefaultPool is a pool created for every new guild.
type DefaultPool struct {
	Slug string
	Name string
	// Entries is optional starter content. It is empty today (new guilds start with
	// blank wing pools they fill themselves), but the field is here so a future
	// default seed can ship starter squares for every guild without changing the
	// bootstrap - EnsureGuild seeds these only into a freshly created, empty pool.
	Entries []string
}

// DefaultPools returns the pools auto-created for a new guild: the eight raid
// wings, blank. They are ordinary pools (equal to any the guild creates later) and
// fully deletable. To ship starter content for all guilds in the future, add
// Entries here (or wire a DEFAULT_SEED_FILE into ApplySeed for a per-guild seed).
func DefaultPools() []DefaultPool {
	return []DefaultPool{
		{Slug: "w1", Name: "Wing 1"},
		{Slug: "w2", Name: "Wing 2"},
		{Slug: "w3", Name: "Wing 3"},
		{Slug: "w4", Name: "Wing 4"},
		{Slug: "w5", Name: "Wing 5"},
		{Slug: "w6", Name: "Wing 6"},
		{Slug: "w7", Name: "Wing 7"},
		{Slug: "w8", Name: "Wing 8"},
	}
}
