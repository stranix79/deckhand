---
title: Deckhand 1.3: a fifteen second demo, comparison pages, and a prompt for your LLM
date: 2026-09-23
summary: Three days after 1.0, a release about explaining rather than adding. What changed on the site and in the binary, and how to update.
image: stage.jpg
credit: Photo Jorge Jaramillo, Wikimania 2026 Paris, CC BY-SA 4.0.
---

The first feedback after the launch was not about features. It was "I don't get what it does" and "how is it different from reveal.js?". Fair. So 1.3 is a release about explaining, with one fix in 1.3.1 that the release pipeline forced on me.

## The demo, in fifteen seconds

The landing page now opens on a screen recording of the three screens: the stage on the left, the remote on a phone, the audience view on the right, with the laser and the QR code in action. No mock-up, the real thing, served by the hub itself (`/static/site/demo.mp4`, with Range support so Safari plays it). The same recording is a [GIF in the README](https://github.com/stranix79/deckhand#readme).

## Deckhand versus the others

Three honest pages, with a table and a "where each one is the better pick" section: [reveal.js](/vs/reveal-js), [Slidev](/vs/slidev) and [Google Slides](/vs/google-slides), plus an [overview](/vs). Short version: they make slides, Deckhand presents the HTML you already have. If you need themes, plugins and PDF export, reveal.js is the better tool. If you need to present a folder from any screen with your phone in your hand, that is Deckhand.

## A prompt for your language model

Most of my decks are generated. So here is [the prompt](/docs/LLM) that makes any model produce a folder that passes `deckhand validate` on the first try: one file per slide, the naming, the `deck.json`, the rules on scripts and assets. Paste it, describe your talk, present.

## Under the hood

- `/sitemap.xml` and `/robots.txt`; docs, changelog and comparison pages carry a description, a canonical URL and Open Graph tags, so they can be indexed and shared with a proper preview. App pages and live screens stay out of search engines.
- 1.3.1: the changelog is now embedded from the repository root. The CI, goreleaser and Homebrew builds never copied it, so their binaries answered 404 on `/changelog` and the release pipeline failed on that very test. Fixed, with a test that checks the exact robots tag instead of the word "noindex" (which the changelog text itself contains).

## Updating

```
brew upgrade deckhand            # Homebrew
deckhand version                 # should say 1.3.1
```

Or grab the binary for your platform in the [releases](https://github.com/stranix79/deckhand/releases). Self-hosted hubs: bump the image tag and `docker compose up -d`, migrations run at start.

Full details in the [changelog](/changelog).
