# CLI

```
deckhand present  <deck> [--port 7777] [--ip 192.168.1.20] [--open] [--no-lan] [--hub https://…] [--slug ship-it]
deckhand validate <deck> [--quiet]
deckhand login    --hub https://… --token …
deckhand push     <deck> [--hub https://…] [--slug ship-it]
deckhand serve    [--addr :8080] [--pg postgres://…] [--deck-origin https://decks.…] [--base-url https://…]
deckhand import   <file.pptx> [-o dir] [--dark] [--force]
deckhand version
```

`<deck>` is a directory, a `.zip` or a `.tar.gz` (see [FORMAT.md](FORMAT.md)).

## `present`

1. Loads and validates the deck (same rules as `validate`).
2. Starts a server on every interface, port 7777 or the next free one.
3. Prints the stage, remote and audience links and two QR codes: scan
   **REMOTE** with your phone, show **AUDIENCE** to the room.
4. `--open` opens the stage in your default browser. Press `f` there for
   fullscreen, `b` for a black screen, `q` to show the audience QR.
5. `--hub` pushes the deck to a hub and relays every state change so people
   outside the LAN can follow (milestone 4). Losing the hub only prints a
   warning; the local presentation is unaffected.

The LAN IP is auto-detected (docker, VPN, VM and link-local interfaces are
skipped). Use `--ip` when the guess is wrong, `--no-lan` to stay on
127.0.0.1.

Exit codes: 0 on Ctrl-C, 1 on an invalid deck or a port that cannot be bound.

## `validate`

Prints title, ratio, slides (● has notes, ◎ has public notes), warnings and
errors. Exit 0 when the deck can be presented, 1 otherwise. `--quiet` prints
only warnings and errors.

## `import`

Converts a PowerPoint file into a deck you can present and edit:

```
deckhand import talk.pptx          # writes ./talk/
deckhand present talk
```

It writes one HTML file per slide, `deck.json` with your presenter notes, and
`assets/` with the pictures, the videos and one `style.css` that is yours to
change. `-o` picks another directory, `--dark` the dark theme. An existing
directory that is not empty is refused unless you pass `--force`; nothing is
ever deleted.

Each slide is converted in one of two ways, decided slide by slide:

* A **text slide** (title, bullets, pictures, tables in the placeholders of
  its layout) becomes clean HTML: an `<h2>`, real nested lists, a `<table>`,
  pictures next to the text. It takes the look of `style.css`, not the look
  of the PowerPoint theme. This is the HTML you would have written by hand.
* A **drawing** (boxes, arrows and labels placed by hand, like an
  architecture diagram) is redrawn where PowerPoint put things: positioned
  shapes with their fills, outlines and text, connectors as SVG lines with
  their arrow heads. It keeps its own colours, also with `--dark`.

What is kept: titles, text with bold, italic, underline, links and
monospace, bullet and numbered lists with their levels, pictures (PNG, JPEG,
GIF, SVG, WebP), embedded videos and sounds, tables with merged cells,
presenter notes, the slide ratio (the closest of 16:9, 16:10 and 4:3).

What is not: the theme and the master decorations (logos, bands), animations
and transitions, charts, SmartArt, freeform shapes, EMF/WMF pictures, linked
(not embedded) media, hidden slides. Every loss is printed as a warning with
its slide number, and stays visible in the slide as a dashed box until you
replace it. The old binary `.ppt` format is not read: save it as `.pptx`
first.

Text fits its boxes best when the fonts of the PowerPoint file are installed
on the machine that presents. Without them the system font is used, a bit
narrower so labels still fit.

Exit codes: 0 when the deck was written and validates, 1 otherwise.

## `login`, `push`, `present --hub`

1. On the hub, *Decks → Get an API token*, then `deckhand login --hub https://deckhand.show --token …`.
   The token is saved in `~/.config/deckhand/config.toml` (`DECKHAND_HUB` /
   `DECKHAND_TOKEN` override it).
2. `deckhand push talk/` validates, zips and uploads the deck; it prints the
   permanent link `/d/{you}/{slug}`. Pushing the same slug again makes a new
   version. `--slug` picks the slug (default: from the title).
3. `deckhand present talk/ --hub https://…` does the push, starts a relayed
   presentation and prints the hub's audience link next to the local ones.
   Remote viewers follow through the hub; the stage and the remote stay local.

## `serve`

Runs the hub; see [HUB.md](HUB.md) for the environment and deployment.

## Terminal output

Colours are used only when stdout is a terminal and `NO_COLOR` is unset.
