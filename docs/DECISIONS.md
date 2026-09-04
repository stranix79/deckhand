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
