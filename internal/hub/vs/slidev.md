# Deckhand vs Slidev

Slidev is a presentation tool for developers built on Vue and Vite: you
write one Markdown file, each `---` starts a slide, and you get a dev server
with hot reload, themes from npm, Shiki code highlighting, Monaco editors
inside slides, drawing, a presenter mode and exports. Deckhand starts from
the other end: the slides are already HTML files, one per slide, and
Deckhand only presents them.

## Authoring

Slidev is an authoring environment. Markdown plus Vue components, layouts
and themes installed from npm, LaTeX, diagrams, click animations, a
built-in notes editor. It needs Node.js and a project with dependencies.

Deckhand has no authoring layer at all. A slide is a complete HTML document
with its own CSS and JS, designed for a fixed 1920x1080 canvas that Deckhand
scales to the screen. An optional `deck.json` gives the title, the ratio and
the presenter notes. There is no Markdown, no components, no theme; you
bring the HTML, or you have a language model write it (the README has a
prompt for that). The upside is that the deck is a folder of files you own
and can open in any browser, with no build step and no `node_modules`.

## Presenting

Slidev's presenter mode lives at `/presenter` on the dev server: current
slide, notes, next slide, a timer, several layouts, a screen mirror mode.
Run `slidev --remote` and the server listens on your LAN address so you can
open the presenter page from a phone; add a password to keep it private.
Pages opened against the same dev server stay in sync, so people on the
network can follow the play view while you drive from the presenter view.

Deckhand's `present` command fills the same three roles from one binary
with no dev server: a stage link for the projector, a remote link (QR code)
for your phone with notes, timer, laser pointer, black screen and audience
QR, and an audience link for the room. The audience follows live on their
own screens, can wander off to another slide and come back to live. It
works offline on the LAN, and the optional Hub relays the presentation to
people outside the room.

## Export and hosting

Slidev exports to PDF, PPTX, PNG and Markdown (through playwright-chromium)
and builds a static single-page app you can host on Netlify, Vercel or
GitHub Pages. Deckhand has no export: the HTML files are the artifact. It
hosts a deck on the Hub (`deckhand push`, permanent link, a week on the free
plan, no limit on Pro), or you self-host the Hub.

## Side by side

| | Deckhand | Slidev |
|---|---|---|
| Source | One HTML file per slide, optional `deck.json` | One Markdown file, Vue components |
| Toolchain | One binary, no Node | Node.js, Vite, npm dependencies |
| Themes and addons | None | npm ecosystem, official and community |
| Code highlighting, Monaco, LaTeX, diagrams | Whatever you put in the HTML | Built in |
| Click animations | Fragments via postMessage | `v-click` and friends, built in |
| Presenter view | Phone remote (notes, timer, next slide, laser) | `/presenter` in the browser, from a phone with `--remote` |
| Audience follows on their device | Yes, LAN link, no account | Yes against the dev server; a static build does not sync across devices by default |
| Drawing on slides | No | Yes |
| Export PDF, PPTX, PNG | No | Yes |
| Static hosting | Hub, hosted or self-hosted | Any static host after `slidev build` |
| Slide isolation | Sandboxed iframe per slide | Slides share one Vue app |
| Offline | Yes | Yes (dev server or build) |
| License | MIT (hub: BSL 1.1) | MIT |

## When to pick Slidev instead

Pick Slidev when you write the slides yourself and want a pleasant developer
workflow: Markdown, hot reload, code blocks that look right, live editors,
drawing, and a PDF or PPTX to send afterwards. Pick it when your team
already lives in Node and Vue, when you want themes you can `npm install`,
or when the deck must be published as a static site with its animations
intact.

## When Deckhand fits

Pick Deckhand when the HTML already exists, or when you would rather have an
AI produce plain HTML than learn a slide DSL. Pick it when the presentation
moment matters more than the editing moment: a room, a projector, your
phone, people following on theirs, no internet, no account. Pick it when you
want each slide sandboxed because you did not write all of it. And pick it when
one binary on a bare laptop is the whole setup.

## Try it

`brew install stranix79/tap/deckhand`, then
`deckhand present examples/ship-it --open`. The format and the prompt for
generating a deck are in the
[README](https://github.com/stranix79/deckhand#readme); the hosted Hub is
at [deckhand.show](https://deckhand.show/).
