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
