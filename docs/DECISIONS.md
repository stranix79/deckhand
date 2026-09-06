# Decisions log

One line per decision not covered by the brief, with the date and why. Newest first.

- 2026-09-04 — `deck.json` slide entries accept an optional `"public": true` flag; public notes are shown to viewers (brief §5), private notes only to the remote. Simplest way to express it inside the existing `slides` array.
- 2026-09-04 — Archives are extracted to a temporary directory (`os.MkdirTemp`) and served from disk, rather than served straight from the zip. Simpler, and the same code path (`os.DirFS`) then serves directories, zips and tarballs.
- 2026-09-04 — Allowed file extensions are a fixed list in `internal/deck/allowed.go`; anything else in a deck is an error (not a warning), because the brief says "only … are accepted" and a warning would be ignored.
- 2026-09-04 — Ratio → height table: 16:9 → width×9/16, 16:10 → width×10/16, 4:3 → width×3/4, rounded to the nearest integer. `width` defaults to 1920.
- 2026-09-04 — Natural sort compares digit runs numerically (so `2-` < `10-`), the rest byte-wise and case-insensitively. No locale collation: decks must sort the same on every OS.
- 2026-09-04 — Repo lives at `~/git/stranix/deckhand`; remote `git@github.com:stranix/deckhand.git` as instructed, created on GitHub by Gilles.

## 2026-09-04 — Repository lives at github.com/stranix79/deckhand

The brief names `github.com/stranix/deckhand`, but no `stranix` GitHub
organisation exists; the personal account is `stranix79` (same as asm-m1).
The module path follows the repository so that `go install` works. Homebrew
tap will be `stranix79/tap`.

## 2026-09-04 — Homebrew formula builds from source, maintained by a script

goreleaser's `brews` needs a PAT on the tap repository in CI. Instead,
`scripts/update-tap.sh <tag>` writes a formula that compiles the tag's
source tarball with Go (like homebrew-core does) and pushes it to
`stranix79/homebrew-tap`. No secret in CI, `brew install --build-from-source`
works the same way.

## 2026-09-04 — Stage needs the token on the hub only

The brief lets the presenter drive from the stage keyboard, so locally
`/s/{code}` needs no token (LAN, session code is random). On the hub the
stage URL is guessable, so `StageNeedsToken` is on there: the stage link
carries `?t=` like the remote.

## 2026-09-04 — State carries `black`, plus two transient ops

The brief's state is `{slide, fragment, pointer, startedAt}`. `black` is in
the state too so a reconnecting stage restores it. "Show the QR" and "reset
the timer" are ops (`qr`, `reset`), not state: nothing to restore.

## 2026-09-04 — Fragment question is answered by the stage

The server cannot know whether a slide has fragments. When a stage is
connected, `next`/`prev` become a question to the stage (`ask`), which
forwards it to the slide via postMessage and answers within 150 ms; the
server falls back to a slide change after 400 ms. Without a stage the slide
changes immediately. Documented in docs/PROTOCOL.md.

## 2026-09-04 — `deckhand login` writes the config file

The brief expects a token in `~/.config/deckhand/config.toml` but does not
say how it gets there. `deckhand login --hub URL --token TOKEN` verifies the
token against `/api/v1/me` and writes the file (0600). The token is created
on the hub (Decks → Get an API token), shown once.

## 2026-09-04 — Permanent links are viewer-only sessions

`/d/{user}/{slug}` embeds the normal viewer in detached mode on a session of
mode `permalink` whose code is stored on the deck. No presenter, no token;
the session is loaded on demand and dropped from memory when idle. Uploading
a new version resets it, so the link always shows the latest version.

## 2026-09-04 — Hosted mode reuses the local screens unchanged

"Present now" on the hub creates a `hosted` presentation: same stage,
remote and viewer pages, the hub is the session server. The stage requires
the token there (see above). Relayed mode (`present --hub`) mirrors the
local state into a `relay` presentation; the CLI ends it on Ctrl-C and the
janitor ends it after 15 minutes of silence.

## 2026-09-04 — Plan is derived, not stored on the user

A user is Pro while a `subscriptions` row is `active` or `trialing`; nothing
is cached on `users`. Stripe webhooks update that row; the free-plan limits
(`DECKHAND_FREE_*`) are read from the environment.

## 2026-09-04 — Stats are replayed from raw events

`viewers_events` keeps join/leave/slide events; the stats page replays them
(audience per slide = joins − leaves at the moment a slide is shown, time per
slide = gaps between slide events). No aggregation table: a presentation has
at most a few thousand events.

## 2026-09-04 — Docs are rendered from the embedded Markdown

`/docs/{FORMAT,CLI,HUB,PROTOCOL,SECURITY}` and `/changelog` render the
Markdown files with goldmark inside the hub's page template, so the docs on
the site are always those of the running version.

## 2026-09-06 — Billing through Odoo Subscriptions, Stripe kept as an option

The brief said "Stripe Checkout, nothing else". CODE79 runs its accounting
in Odoo 19 Enterprise, which already has Stripe as a payment provider and
the Subscriptions app. So `DECKHAND_BILLING=odoo` makes Odoo the
subscription engine (card storage, monthly renewal, invoices, dunning,
customer portal) and the hub only mirrors "who is Pro" every 5 minutes over
XML-RPC, matched by e-mail. The Stripe backend stays for self-hosters
(`DECKHAND_BILLING=stripe`). Product: "Deckhand Pro", 9 € excl. VAT / month.
