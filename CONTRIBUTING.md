# Contributing

Thanks for your interest in **GW2 Raid Bingo** - a Discord bot and website for Guild Wars 2 raid
bingo. This repository is the public, read-only mirror of the application source, so people can read
and audit the code. Issues and pull requests are welcome here and are reviewed by the maintainer.

## Reporting a bug or requesting a feature

Open an [issue](https://github.com/Syndic0r/gw2-raid-bingo/issues/new/choose) and pick a template. For
bugs, include what you did, what you expected, and what happened (a screenshot helps).

## Development

One Go binary (Go 1.23+) runs both the Discord bot and the web server.

```bash
gofmt -l .        # formatting (should print nothing)
go vet ./...      # static checks
go build ./...    # build
go test ./...     # tests
```

Please keep all of those green in your PR. Match the surrounding style: clear names, comments that
explain *why*, and a test for any behaviour change. The architecture, the bot commands, and the
website are described under `docs/`.

## Scope

This repo is **application code only** (`cmd/`, `internal/`, `docs/`). Hosting, deployment, the card
seed data, and the marketing site are managed privately and aren't part of it - so there are no
secrets, infra, or CI to run here (Actions are disabled on this mirror). Merged changes are pulled
back into the maintainer's private source of truth and deployed from there.

## License

By contributing, you agree your contributions are licensed under the repository's
[LICENSE](LICENSE).
