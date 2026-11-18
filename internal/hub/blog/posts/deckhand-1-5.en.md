---
title: Deckhand 1.5: import your PowerPoint, and install in one line
date: 2026-11-18
summary: Deckhand presents HTML slides, and most slides are still in a .pptx. There is now a command for that. What it keeps, what it loses, and a shorter way to install.
image: import.jpg
---

Deckhand presents a folder of HTML slides. Fine if your slides are already HTML. Most are not. They sit in a .pptx, sometimes since ten years, and "just redo them" is not an answer I would accept myself.

So 1.5 has one new command.

## deckhand import

```
deckhand import talk.pptx     # writes ./talk/
deckhand present talk
```

You get one HTML file per slide, a `deck.json` with your presenter notes, and an `assets/` folder with the pictures, the videos and one `style.css`. That stylesheet is yours, change it. What comes out is a normal deck: you can edit it by hand, run `deckhand validate` on it, push it to a hub.

## Two kinds of slides

My first version read the titles and the bullets and wrote clean HTML. On text slides it was good. Then I gave it a real file, a deck full of architecture diagrams, and I got page after page of bullet lists, with the labels of the boxes one under the other. Useless.

So the importer now decides slide by slide.

- A **text slide** (a title, bullets, a picture, a table) becomes the HTML you would have written by hand: a heading, real nested lists, a real `<table>`. It takes the look of `style.css`, not the look of the PowerPoint theme.
- A **drawing** (boxes, arrows and labels placed by hand) is redrawn where PowerPoint put things: the shapes with their fills and outlines, the text inside, the connectors as SVG lines with their arrow heads. It keeps its own colours, also with `--dark`.

Also kept: bold, italic and links, the levels of the lists, tables with merged cells, embedded videos and sounds, and the slide ratio.

## What is lost

I prefer to write it here than you find it out in front of a room.

Not converted: the theme and the decorations of the master (logos, bands), animations and transitions, charts, SmartArt, freeform shapes, EMF and WMF pictures, media that is linked and not embedded. Hidden slides are skipped. The old binary `.ppt` is not read, save it as `.pptx` first.

Nothing disappears without a word. Each loss is printed with its slide number, and it stays in the slide as a dashed box until you replace it.

One more thing about fonts. Text fits its boxes best when the fonts of the file are installed on the machine that presents. Without them the system font is used, a bit narrower so the labels still fit.

All of this is the Go standard library. No LibreOffice, no Python, nothing to install next to the binary. The details are in the [CLI doc](/docs/CLI).

## Install in one line

```
curl -fsSL https://deckhand.show/install.sh | sh
```

For macOS and Linux. The script takes the binary of the latest release for your machine, checks its SHA-256 against the checksums of the release, and puts it in `/usr/local/bin` or `~/.local/bin`. No sudo. [Read it](/install.sh) before you pipe it, it is about a hundred lines.

Windows gets a zip, and Homebrew still works. Everything is on the new [download page](/download). The landing page sends you there now, it used to send you to GitHub.

## Also since the last post

- Bots were using the sign-in form to send magic links to strangers. The form now has a honeypot, a signed timestamp, and a budget per IP and per address. A refused request gets the normal "check your inbox" page, so a bot learns nothing. (1.4.7)
- `/llms.txt` and `/llms-full.txt`, for the language models that read the site. (1.4.5)

## Updating

```
brew upgrade deckhand            # Homebrew
deckhand version                 # should say 1.5.1
```

Self-hosted hubs: bump the image tag and `docker compose up -d`, migrations run at start.

I tested the import on my own files, and they are not yours. If it breaks on one of your decks, that file is exactly what I need: an [issue on GitHub](https://github.com/stranix79/deckhand/issues) or a mail.

Full details in the [changelog](/changelog).
