# Generate a deck with an LLM

Deckhand does not care who wrote the HTML. A folder of slides written by
hand, by a script or by a language model presents the same way. This page
gives you a prompt that makes Claude, ChatGPT, Gemini or a local model
(Ollama, LM Studio...) produce a complete deck that passes `deckhand
validate` on the first try.

## The prompt

Copy it, replace the three placeholders in the first line, paste it into the
model. Ask for the files one by one if the model truncates long answers.

```
Create a slide deck for Deckhand (https://deckhand.show) about TOPIC, for AUDIENCE, in about N slides.

Output a folder named my-talk/ with one HTML file per slide plus a deck.json manifest. Rules:

1. File names: 01-title.html, 02-<short-name>.html, ... up to the last slide. Two-digit prefix, lowercase, hyphens, .html extension. The first slide is a title slide, the last one a closing slide with a one-line takeaway.
2. Each file is a complete, self-contained HTML document: <!doctype html>, <html lang="en">, <meta charset="utf-8">, a <title>, all CSS inside a <style> tag in the head, any JavaScript inside a <script> tag. No external resources at all: no CDN, no Google Fonts, no <link href="https://...">, no <img src="https://...">, no fetch or XMLHttpRequest. Slides run inside a sandboxed iframe (sandbox="allow-scripts", no allow-same-origin) and must work offline, so nothing outside the folder will load, and there is no localStorage, no cookies, no form submission, no window.open and no navigation of the parent window. Draw illustrations with inline SVG or CSS.
3. Each slide is designed for a fixed 1920x1080 canvas (16:9). Set html and body to width: 1920px; height: 1080px; margin: 0; overflow: hidden. Use pixel sizes, never vw, vh or viewport units: Deckhand scales the whole page to fit the screen. Keep text big: titles 88px or more, body text 40px or more, at most six lines of text per slide. Use system fonts (font-family: system-ui, sans-serif).
4. deck.json at the root of the folder, exactly this shape and no other keys:
{
  "title": "Deck title",
  "ratio": "16:9",
  "slides": [
    { "file": "01-title.html", "notes": "What to say on this slide, two to four sentences of plain text." },
    { "file": "02-short-name.html", "notes": "..." }
  ]
}
List every slide file in order. Every entry has "file" and "notes" (presenter notes, plain text, shown on the presenter's phone only). Add "public": true to an entry when its notes should also be shown to the audience. Unknown keys are an error.
5. One idea per slide, consistent colours and typography across all slides, high contrast.

Print each file in its own code block, preceded by its path.
```

After the model answers, save the files, then:

```
deckhand validate my-talk/     # lists every problem, exit code 1 if the deck cannot be presented
deckhand present  my-talk/     # stage, remote and audience links, QR codes
```

## Why these rules

Every line of the prompt maps to something Deckhand checks or to how it
renders slides (full details in [FORMAT.md](FORMAT.md) and
[SECURITY.md](SECURITY.md)):

| Rule in the prompt | Reason |
|---|---|
| Numbered file names | Without a manifest, slides are the root `*.html` files in natural order. With one, the order is the `slides` list, but numbered names keep the folder readable. |
| Self-contained HTML, no external resources | Slides run in `<iframe sandbox="allow-scripts">`, never `allow-same-origin`, and the deck is served with `Content-Security-Policy: sandbox allow-scripts`. Presentations must work on a LAN with no internet. Files the deck needs (CSS, JS, images, fonts) may live inside the folder and be linked with relative paths. |
| No storage, forms, popups, parent navigation | All blocked by the sandbox. A slide that relies on them breaks silently. |
| Fixed 1920x1080, pixel units | Deckhand scales the page to the screen; viewport units would fight that scaling. `ratio` accepts `16:9`, `16:10` or `4:3`, and `width` (default 1920) sets the design width. |
| `deck.json` with `file` and `notes` only | The manifest is decoded strictly: a typo such as `"slide"` for `"slides"` or an extra `"speaker_notes"` key fails validation instead of being ignored. |
| `public: true` | Shows the notes of that slide under it on the audience's phones. |

## What the model may add

* **Fragments** (reveal step by step inside a slide): the slide answers
  `deckhand:next` / `deckhand:prev` messages with `postMessage`. Paste
  `examples/ship-it/assets/fragments.js` into the prompt and ask the model
  to use it. Protocol in [FORMAT.md](FORMAT.md).
* **Notes from the slide itself**: a slide may post
  `{ type: "deckhand:notes", text: "..." }` and override the manifest.
* **Shared assets**: an `assets/style.css` linked with a relative path from
  every slide is fine and keeps the files small. Allowed file types are
  listed in [FORMAT.md](FORMAT.md).
* **Photos**: only from files inside the folder. If the model invents URLs
  for images, remove them or download the images into `assets/`.

## Typical failures and the fix

| `deckhand validate` says | Fix |
|---|---|
| `deck.json: invalid JSON: json: unknown field "speaker_notes"` | Rename it `notes`, or delete it. |
| `deck.json: slides[3] file "04-demo.html" not found in the deck` | The model listed a file it never printed. Ask for it, or remove the entry. |
| `deck.json: unknown ratio "16/9" (use "16:9", "16:10" or "4:3")` | Use `"16:9"`. |
| `evil.exe: file type not allowed (allowed: ...)` | Only the extensions listed in FORMAT.md may be in the folder; delete the file. |
| Slide renders tiny or cropped | The page uses `vw`/`vh` or a body size other than 1920x1080. |
| Font falls back on the stage | A Google Fonts `<link>` slipped in. Use system fonts or ship the `.woff2` inside the deck. |
