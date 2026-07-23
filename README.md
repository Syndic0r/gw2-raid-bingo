# GW2 Raid Bingo

A free Discord bot and website for **Guild Wars 2 raid bingo**. Open a game from any set of square
pools you choose, and every player gets their own live 5x5 card; mark your squares as the run
unfolds - in Discord or in the browser, kept in sync - and the first to call **BINGO** wins the round.

- **Add the bot to your server:** https://gw2-raid-bingo.duckdns.org
- **Play:** https://gw2-raid-bingo.duckdns.org

This repository is the **public source mirror** of the application - the Discord bot, the web server,
and the game engine - so anyone can read and audit the code. It is published automatically from the
maintainer's private repository; the private repo additionally holds deployment scripts, the hosted
instance's data, and the marketing site, none of which are needed to understand or run the bot.

> A source mirror lets you see *what the code does*. Like any hosted bot, it cannot prove that the
> instance you add to your server runs exactly this code. If you want full certainty, self-host it.

## What it does

- One **randomly dealt 5x5 card per player**, drawn from your server's own bingo squares, with a free
  centre. Standard bingo: first to complete a row, column, or diagonal calls it.
- **Discord and web in sync** over one live game - toggle a square in either place and it updates in
  the other.
- Games built from **any combination of square pools** you pick (new servers start with blank
  Wing 1-8 pools), opened on demand or **scheduled** to start at raid time.
- **Win celebration** (with confetti on the website), a leaderboard, and game history.
- Each server writes its **own squares** with `/bingo-data` (import/export supported).

## Permissions and privacy

- No privileged Discord intents. The install asks only for: View Channels, Send Messages, Embed
  Links, Attach Files, Read Message History.
- No message content is read or stored; no tracking, ads, or analytics.
- It stores the minimum to run a game: Discord IDs, your card and marks, server configuration, and the
  squares your admins add. See the hosted instance's
  [Privacy Policy](https://gw2-raid-bingo.duckdns.org/privacy.html) and
  [Terms](https://gw2-raid-bingo.duckdns.org/terms.html).

## Building it

One Go binary (Go 1.23+) runs both the bot and the web server:

```
go build ./cmd/gw2bingo
```

Configuration is read from the environment (see `gw2-raid-bingo.conf.example`); `docs/` describes the
architecture, the bot commands, and the website. The card texts are provided per server via the bot -
none ship in this repository.

## Credits

The original bingo card idea and starter square lists come from
[ARThinx/GuildWars2Bingo](https://github.com/ARThinx/GuildWars2Bingo).

## License

MIT - see [LICENSE](LICENSE).
