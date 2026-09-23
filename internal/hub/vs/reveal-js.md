# Deckhand vs reveal.js

reveal.js is the reference for HTML presentations: an open source framework
(MIT) around since 2011, with a large ecosystem of plugins, themes and
examples. Deckhand is not a framework. It is a Go binary that takes a folder
of HTML files, one per slide, and gives you a stage, a phone remote and a
live audience link. The two overlap on "slides are HTML" and diverge on
almost everything else.

## What each one is

With reveal.js you write one `index.html` where every `<section>` is a
slide, load the library and a theme, and it handles navigation, transitions,
fragments, vertical slides, auto-animate, code highlighting, Markdown, math,
speaker notes and PDF export. Your deck depends on the library, its CSS and
the plugins you picked.

With Deckhand each slide is a complete HTML document. No library to load, no
`<section>`, no theme system. Deckhand renders every slide in a sandboxed
iframe (`sandbox="allow-scripts"`, never `allow-same-origin`), scales it to
the screen and offers an optional `postMessage` protocol for fragments and
notes. A plain HTML page written by hand or by a language model is already a
valid slide.

## Presenter and audience

reveal.js has a speaker view: press `S` and a second browser window opens
with the notes, the next slide, a timer and a pacing timer. It requires the
deck to be served by a web server, and the window lives on the machine that
presents. To put the notes on another device you run the separate Node
notes server plugin. To let the audience follow on their own devices you use
multiplex, a plugin that left the core in reveal.js 4.0 and needs a
socket.io server; the public demo server comes with no guarantee and the
docs recommend running your own.

Deckhand's `present` command starts one server on your machine and prints
three links and two QR codes. The phone remote shows the current and next
slide, the notes and a timer, and drives prev/next, a laser pointer (drag on
the thumbnail), a black screen and an audience QR on the stage. The audience
link works for everyone on the LAN, with no account and no app; people
follow live, browse on their own and come back to live. Nothing leaves the
room unless you connect the optional Hub for viewers outside the LAN and
permanent links.

## Side by side

| | Deckhand | reveal.js |
|---|---|---|
| Slide source | One self-contained HTML file per slide | `<section>` elements in one page, or Markdown |
| Dependency inside the deck | None | reveal.js, a theme, plugins |
| Transitions, auto-animate, vertical slides | No (your own CSS per slide) | Yes, built in |
| Fragments | Yes, small postMessage protocol | Yes, built in |
| Themes | None | 12 built in, many more from the community |
| Speaker notes and timer | On the phone remote | Speaker view window (`S` key) |
| Remote from a phone, no app | Yes, QR code | Not built in; community plugins |
| Audience follows on their device | Yes, LAN link, no account | multiplex plugin plus a socket.io server |
| PDF export | No | Yes (`?print-pdf`, decktape) |
| Slide isolation | Sandboxed iframe, strict CSP | None, slides share the page |
| Install | One binary | npm or a script tag |
| Hosted option | Hub at deckhand.show, optional | slides.com, a separate product |
| License | MIT (hub: BSL 1.1) | MIT |

## When to pick reveal.js instead

Pick reveal.js when you want a mature toolkit for making slides: themes,
auto-animate, vertical navigation, syntax highlighting, math, Markdown, a
PDF at the end, and thousands of examples to copy from. If you already have
a reveal.js deck and only miss a phone remote, a community plugin gets you
there with less change than moving to Deckhand. Pick it too when you need to
embed the deck in another page, or when the slides must also read as a
scrollable document.

## When Deckhand fits

Pick Deckhand when your slides already exist as HTML pages, typically
written by an AI or generated from data, and you want to present them
without rewriting them into a framework. Pick it when the phone remote and
the audience link matter more than transitions, when the room has no
internet, or when you do not want slide code to run with access to your
page (a deck you did not write, a demo with third-party snippets). And pick
it when "install" has to mean one file.

## Try it

`brew install stranix79/tap/deckhand`, then
`deckhand present examples/ship-it --open`. The format and the prompt for
generating a deck are in the
[README](https://github.com/stranix79/deckhand#readme); the hosted Hub is
at [deckhand.show](https://deckhand.show/).
