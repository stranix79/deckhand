# Examples

## Field notes — the showcase

Seventeen slides that make the case for HTML over PowerPoint, with full-bleed
photos, a section of slides that **are running code** (a live clock and a
canvas particle field, a slider that drives an SVG chart, a git diff of a
slide, a scorecard), fragments, an animated chart, and a photo grid.

* **Browse it live**: [deckhand.stranix.net/d/stranix79/field-notes](https://deckhand.stranix.net/d/stranix79/field-notes)
* **Source**: [examples/keynote on GitHub](https://github.com/stranix79/deckhand/tree/main/examples/keynote)
  (photo credits in `CREDITS.md`).
* Present it: `deckhand present deckhand/examples/keynote --open`. On slide 5,
  drag the slider on the stage or use ↑/↓.

## Ship it before lunch — the minimal one

The deck that ships with the repository: eight slides, speaker notes, two
public notes, and one slide with **fragments** (step-by-step reveal driven by
the remote).

* **Browse it live**: [deckhand.stranix.net/d/stranix79/ship-it-before-lunch](https://deckhand.stranix.net/d/stranix79/ship-it-before-lunch)
  (viewer in detached mode: swipe or use the arrow keys).
* **Source**: [examples/ship-it on GitHub](https://github.com/stranix79/deckhand/tree/main/examples/ship-it)
  — `deck.json`, the eight HTML files, a shared `assets/style.css` and the
  40-line `assets/fragments.js` that implements the slide protocol.

### Present it on your machine

```
brew install stranix79/tap/deckhand          # macOS / Linux (builds with Go)
git clone https://github.com/stranix79/deckhand
deckhand present deckhand/examples/ship-it --open
```

The stage opens in your browser (press `f` for fullscreen). The terminal
prints two QR codes: scan **REMOTE** with your phone to get prev/next, the
notes, the timer and the laser pointer; share **AUDIENCE** with the room.
Slide 3 has fragments: each *next* reveals one command before moving on.

No Homebrew? Download a binary from the
[releases](https://github.com/stranix79/deckhand/releases) and run the same
command.

### Publish it on the hub

```
deckhand login --hub https://deckhand.stranix.net --token …   # token from Decks → Get an API token
deckhand push deckhand/examples/ship-it
```

You get a permanent link like the one above, under your own handle.

## Make your own

Copy the folder, replace the HTML files, keep `assets/style.css` if you
like the look. Every slide is a standalone page designed at 1920×1080; the
rules are in [FORMAT.md](FORMAT.md). `deckhand validate my-talk/` tells you
what is wrong before you present.

Ask your AI for "one HTML file per slide, 1920×1080, self-contained, plus a
deck.json with notes": that is exactly the format.
