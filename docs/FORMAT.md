# Deck format

A **deck** is a directory. You can also hand Deckhand a `.zip` or a `.tar.gz`
of that directory; it is extracted to a temporary folder and treated the same.

```
my-talk/
├── deck.json          optional
├── 01-title.html      one file per slide
├── 02-problem.html
├── 03-demo.html
└── assets/            anything the slides reference, any depth
    ├── style.css
    └── logo.svg
```

## Slides

* One **HTML file per slide**. The file must be self-contained: it can link
  CSS, JS, images, fonts and videos **inside the deck** with relative paths.
  Anything outside the deck (a CDN, Google Fonts) may not load: slides run in a
  sandboxed iframe and presentations must work offline.
* Without `deck.json`, the slides are every `*.html` / `*.htm` file **at the
  root** of the deck, in **natural order**: `2-intro.html` comes before
  `10-end.html`, `a.html` before `B.html`. Digit runs are compared as numbers,
  the rest byte-wise, case-insensitively, regardless of the OS locale.
* A slide is designed for a fixed size (default 1920×1080) and Deckhand scales
  the whole page to fit the screen. Use absolute pixel sizes in your CSS, not
  `vw`/`vh`.

## `deck.json`

```json
{
  "title": "Ship it before lunch",
  "ratio": "16:9",
  "width": 1920,
  "slides": [
    { "file": "01-title.html", "notes": "Wait for the room to settle." },
    { "file": "02-problem.html", "notes": "Ask who uses AI for slides.", "public": true },
    { "file": "03-demo.html" }
  ]
}
```

| Field | Default | Meaning |
|---|---|---|
| `title` | directory name | Shown on the remote, the viewer and the Hub. |
| `ratio` | `"16:9"` | `"16:9"`, `"16:10"` or `"4:3"`. Anything else is an error. |
| `width` | `1920` | Design width in CSS pixels. Height is derived: 16:9 → 1080, 16:10 → 1200, 4:3 → 1440 (for 1920). |
| `slides` | all root HTML files | Ordered list. When present it is **authoritative**: files not listed are not slides (they may still be assets). |
| `slides[].file` | required | Path relative to the deck root, forward slashes, no `..`. Must exist and end in `.html`/`.htm`. |
| `slides[].notes` | `""` | Presenter notes, plain text. Shown on the remote only. |
| `slides[].public` | `false` | Also show these notes to the audience (under the slide, on phones). |

Unknown fields are an **error**, so a typo (`"slide"` for `"slides"`) is
caught instead of silently ignored.

## Limits and refusals

`deckhand validate`, `deckhand present`, `deckhand push` and the Hub upload all
go through the same loader, so what validates locally presents everywhere.

| Rule | Value |
|---|---|
| Total size | 200 MB, counted on the **uncompressed** files |
| Slides | 500 max |
| Files | 5 000 max |
| Allowed file types | `.html .htm .css .js .mjs .json .png .jpg .jpeg .gif .webp .avif .svg .ico .mp4 .webm .mov .mp3 .ogg .wav .m4a .woff .woff2 .ttf .otf .pdf .txt .md .vtt` |
| Refused | any other extension, symbolic links, archive entries with `..`, absolute paths, backslashes or drive letters (zip slip) |
| Ignored | `.DS_Store`, `__MACOSX`, `Thumbs.db`, `._*` |

Archives that wrap everything in a single top-level folder (what you get when
you zip a directory on macOS or Windows) are unwrapped automatically.

`validate` reports **every** problem, not just the first one, and exits with
code 1 when the deck cannot be presented. Warnings (no `deck.json`, empty
files) do not fail the deck.

## Optional protocol: slide ↔ Deckhand

A slide that does nothing special works as-is. A slide that wants
**fragments** (reveal step by step inside one slide) or wants to **provide its
notes** talks to Deckhand with `postMessage`. See
`examples/ship-it/assets/fragments.js` for a complete implementation.

Deckhand → slide, on every prev/next request:

```js
{ type: "deckhand:next" }
{ type: "deckhand:prev" }
```

The slide answers on `event.source`:

```js
{ type: "deckhand:handled", handled: true }   // I moved a fragment, stay here
{ type: "deckhand:handled", handled: false }  // nothing left, change slide
```

If the answer is `false` or nothing arrives within **150 ms**, Deckhand changes
slide. Slide → Deckhand, at any time:

```js
{ type: "deckhand:ready" }                       // optional, on load
{ type: "deckhand:notes", text: "Presenter notes" } // overrides deck.json notes
```

Use `"*"` as the target origin in both directions: the stage and the slide are
served from different origins on purpose (sandbox).
