# Deckhand

**Turn a folder of HTML slides into a presentation**: a stage on the big
screen, a remote on your phone, and a live link the audience opens on theirs.

Your AI already writes one HTML file per slide. Deckhand is what presents them.

```
deckhand validate talk/     # check the deck, list every problem
deckhand present  talk/     # stage + remote + viewer on your LAN     (milestone 3)
deckhand push     talk/     # publish on a Hub, get a public link      (milestone 5)
```

Status: **milestone 1 of 6**. `validate`, the deck format and the example deck
exist. `present`, the three screens and the Hub are next. Roadmap in
[CLAUDE.md](CLAUDE.md), decisions in [docs/DECISIONS.md](docs/DECISIONS.md).

## Install

Binaries come with milestone 3 (goreleaser). Until then, with Go 1.23+:

```
git clone https://github.com/stranix79/deckhand && cd deckhand
make build          # → ./deckhand
./deckhand validate examples/ship-it
```

## The deck format

A directory, or a `.zip`/`.tar.gz` of it. One HTML file per slide, natural
order, optional `deck.json` for title, ratio and presenter notes. Slides may
opt into fragments with a tiny `postMessage` protocol. Everything is in
[docs/FORMAT.md](docs/FORMAT.md); [examples/ship-it](examples/ship-it) is an
eight-slide deck that uses all of it.

## Development

```
make test       # go test -race ./...
make lint       # golangci-lint
make validate   # build + validate the example deck
```

## License

MIT for everything, except `internal/hub` (the hosted multi-tenant part) which
is under the Business Source License 1.1, see [LICENSE.hub](LICENSE.hub).
Made in Belgium by [CODE79](https://stranix.net).
