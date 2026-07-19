# Discord bot

One bot per Discord application, usable in any number of servers (multi-tenant).
It uses no privileged intents.

## Commands

| Command | Who | What |
|---|---|---|
| `/setup` | Server administrators | Pick the announcement channel (required - where win messages post), the bingo-admin roles (multi-select), and an optional participant role pinged on a win. Each choice saves immediately. |
| `/bingo new instance [replace]` | Bingo admins | Open a game for an instance. All shared pools are included (fine-grained pool selection is a website feature). If one is open, `replace` (or the confirm button) aborts it first. |
| `/bingo abort instance` | Bingo admins | Abort the open game after a confirm; its cards become read-only. |
| `/bingo card instance` | Anyone | Get your own card for the open game as an image plus a 5x5 grid of toggle buttons. Completing a line offers a CALL BINGO follow-up. |
| `/bingo status instance` | Anyone | Show player count, closest-to-bingo leaders, and the website link. |
| `/bingo post instance` | Bingo admins | Post a live status message with a "Deal me in" button; it updates as the game changes. |
| `/bingo cards instance` | Bingo admins | Read-only inspection: pick a player, view their card image. |
| `/bingo schedule [in] [at] [tz] [replace]` | Bingo admins | Schedule games to open later. Give a delay (`in:2h30m`, `1d`) or an absolute time (`at:2026-07-20 20:00`, optional `tz:Europe/Berlin`), then pick one or more instances from the multi-select. A background scheduler opens them at that time and announces it. |
| `/bingo scheduled` | Bingo admins | List upcoming scheduled games with their ids. |
| `/bingo unschedule id` | Bingo admins | Cancel a scheduled game by id. |
| `/bingo-data pool-add\|add\|list\|remove\|clear\|import\|export` | Bingo admins | Manage this server's card texts. The `pool` option autocompletes every pool (static wings/encounters + shared, labeled by type), so there is no guessing a slug. `clear` empties a whole pool (with confirm); `import`/`export` move the whole data set as a JSON file. |

## Scheduling

`/bingo schedule` resolves the time first (a relative duration or an absolute
date-time, validated to be in the future and within 60 days), then presents an
instance multi-select so one schedule can open several wings at once. The
resolved fire time rides in the select's custom id, so the flow needs no stored
interaction state. A scheduler loop in the bot ticks every 30 seconds, atomically
claims due entries (so a game is never opened twice), opens each game with all
shared pools, refreshes the status message, and posts a heads-up to the
announcement channel. A schedule whose instance already has a game open is
skipped unless it was created with `replace`.

## Setup requirements

An announcement channel is required before any game can be opened or scheduled:
it is where a game start and a win are both announced, so a game always has
somewhere to post. Games themselves can be started from any channel. The optional
participant role, if set, is pinged when a game starts and when a game is won, so
everyone playing is kept in the loop; the winner is always pinged on a win too
(and only those are ever pinged - never `@everyone`).

## Bingo-admin rule

A member is a bingo admin if they are the guild owner, hold a role with Discord's
Administrator permission, or hold one of the configured bingo-admin roles.
`/setup` itself requires Discord Administrator. This rule is defined once in the
`authz` package and enforced in the `service` layer.

## Cards in Discord

A card is 25 buttons across five action rows (Discord's maximum), so the grid
uses the whole message. The centre is a disabled free space; marked squares turn
green. Because there is no sixth row for a CALL BINGO button, that control is
sent as an ephemeral follow-up the moment a card completes a line. Personal cards
are ephemeral (only the player sees them); the public status message is the
shared, live-updating surface.

## Celebration

When a player calls bingo, the game finishes atomically (first caller wins), all
cards lock, and the bot posts a celebration - message, winning card image, and
stats - to the configured announcement channel.
