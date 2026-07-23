# Architecture

The service is one Go binary (`app/cmd/gw2bingo`) that runs both the Discord bot
and the web server over a shared store. The packages below are the core; the bot
(`internal/discord`) and web (`internal/web`) layers sit on top of them.

## Packages (`app/internal/`)

### `bingo`
Pure game domain, no I/O. Deals a 5x5 card by sampling 24 squares uniformly at
random from the union of the selected pools' entries (plus the free centre), and
detects completed lines. All randomness is injected, so generation is
deterministic and unit-tested.

### `authz`
The single "is this member a bingo admin" rule, as a pure function shared by the
bot and the web API: admin when the member is the guild owner, holds a role with
Discord's Administrator permission, or holds a configured bingo-admin role.
`/setup` itself is gated on Discord Administrator only.

### `store`
SQLite persistence and the transactional game logic, shared by the bot and web.
Every query is parameterized; no SQL is built by string concatenation. Data is
guild-scoped. Highlights:

- Schema in `migrations/*.sql`, applied in order and recorded in
  `schema_migrations`. STRICT tables; foreign keys and WAL enabled per
  connection.
- Guilds, settings, and configured admin roles; eight blank wing pools
  (`w1`-`w8`) are created for a new guild automatically. All pools are equal,
  guild-editable, and deletable.
- Entries are soft-deleted so historical cards, which snapshot their text, are
  never disturbed. Per-guild and per-pool caps bound abuse.
- A game is defined by the set of pools it draws from; its `pool_set_key` (the
  canonical sorted pool-id set) is its identity. One open game per (guild,
  pool-set), enforced by a unique partial index. Cards are dealt one per user per
  game.
- `CallBingo` finalizes a win in a single transaction guarded by
  `status = 'open'`, so the first caller wins any race and the game and all its
  cards become read-only.

### `events`
A small in-process publish/subscribe hub. Every state change is published once
and fanned out to interested subscribers: the Discord live-message updater
(global subscription) and the web server's SSE connections (per-game topics).
Delivery is non-blocking so a slow subscriber never stalls a game.

### `config`
Runtime configuration loaded from the environment (a systemd EnvironmentFile on
the host). Secrets live only in the environment, never in the repo or database.

### `render`
Draws a bingo card to a PNG for Discord, using the BSD-licensed Go font bundled
in `golang.org/x/image` (no font file shipped).

### `service`
The shared application layer over the store and event hub. Every game action the
bot and the web server perform goes through here, so the single bingo-admin rule
and event publishing are enforced in one place. Role resolution
is injected (`RoleResolver`), implemented over the Discord REST API in
production and faked in tests.

### `discord`
The bot: registers slash commands and routes interactions (commands and
button/select components) to the service. It holds no game logic. Cards are shown
as a rendered image plus a 5x5 grid of toggle buttons (Discord's exact
five-row limit); the CALL BINGO control arrives as a follow-up when a line
completes. A global event subscription keeps the public status message current
and fires the win celebration. No privileged intents are used - member roles are
fetched via REST - which keeps Discord verification simple as the bot scales.

### `web`
The website: a public landing page with the bot invite, Discord OAuth login
(authorization-code + PKCE), a server picker (the user's guilds intersected with
the bot's), and playable cards synced live over SSE. It shares the store,
service, and hub with the bot, and reads the bot's guild membership through the
`BotPresence` interface. Sessions are stored as token hashes; a strict CSP and
same-origin, self-contained assets keep the attack surface small. See
docs/03-website.md.

### Seeding
`store.ApplySeed` loads the private seed file into the configured home guild
only, idempotently (a pool that already has entries is left alone). The seed
file lives outside `app/` and is never embedded, so it cannot reach the public
mirror. Any other guild builds its own data via `/bingo-data` (including bulk
`import`/`export`).
