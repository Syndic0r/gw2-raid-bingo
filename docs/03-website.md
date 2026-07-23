# Website

The website shares the store, service, and event hub with the bot in one
process, so a toggle on the site and a toggle in Discord update each other live.
It is a small vanilla single-page app served from an embedded filesystem; there
is no build step and no external resources (a strict `default-src 'none'` CSP is
enforced).

## Pages and flow

- **Landing (public):** what the bot is, an "Add the bot to your server" button
  (`/invite`), a quick-start, and "Log in with Discord". Marked `noindex`.
- **Login:** Discord OAuth2 authorization-code flow with PKCE and a `state`
  parameter (both carried in short-lived HttpOnly cookies) over the `identify`
  and `guilds` scopes. The access token is used once, server-side, to read the
  user's identity; it is never persisted.
- **Server picker (multi-server):** after login the site shows the user's servers
  that the bot is also in - the user's guild list intersected with the bot's. One
  server auto-selects; several show a picker; none prompts to add the bot.
- **Games list:** the guild's open games (each labelled by its pool set), plus,
  for admins, a **New game** button that opens a pool multi-select (with an
  optional custom name). Selecting a game shows a live board with
  deal/toggle/CALL BINGO, admin abort, and a leaderboard. Confetti fires on a win.

## Managing data (admins)

Bingo admins get a **Manage data** button: a page listing every pool (all equal
and deletable now - new servers start with blank Wing 1-8 pools) with its
squares. From there they add, edit, or remove squares and create or delete pools,
with no ids to guess. Every data endpoint re-checks membership and the
bingo-admin rule server-side, and all store queries are guild-scoped, so an admin
of one server can never touch another server's data.

## Live updates

`GET /api/guild/{id}/events?game=<id>` is a Server-Sent Events stream scoped to
one game. On each event the client refetches the board, so payloads stay tiny
and a missed event self-heals on the next fetch or reconnect. `EventSource`
reconnects automatically.

## Security

- Sessions are a 256-bit random token in an HttpOnly, Secure (on HTTPS),
  SameSite=Lax cookie; only the token's SHA-256 hash is stored, so a database read
  cannot reveal a usable session.
- Every guild-scoped endpoint verifies the bot is in the guild and the user is a
  member (resolved via the bot token); admin actions additionally require the
  bingo-admin rule. Authorization is enforced in the shared service layer, the
  same code the bot uses.
- All game text is inserted with `textContent`, never `innerHTML`, so card texts
  cannot inject markup. Request bodies are size-capped and reject unknown fields.
- The client secret and bot token live only in the environment, never in the repo
  or database.

## Configuration

Web mode activates when `DISCORD_APP_ID`, `DISCORD_CLIENT_SECRET`, and
`BASE_URL` are all set (see `gw2-raid-bingo.conf.example`); otherwise
the bot runs standalone. `BASE_URL` is the origin (no path); `BASE_PATH` is the
path prefix the app is mounted under (unset/`/` = the root; production sets
`/play`). The OAuth redirect URI is `BASE_URL` + `BASE_PATH` + `/auth/callback`
(e.g. `https://gw2-raid-bingo.duckdns.org/play/auth/callback`), which must be
registered on the Discord application exactly.
