---
title: Deckhand 1.0 is out. Your slides are already web pages, present them like it.
date: 2026-09-22
summary: One Go binary, a folder of HTML slides, three screens. Why I wrote it, what it does, and what I would love to hear from you.
image: launch.jpg
credit: Photo SpaceX, JCSAT-16 launch, CC0.
---

For years my talks depended on an HDMI cable, a Keynote that decides not to open, and the ritual question "do you have a PDF, just in case?". I do infrastructure for a living. I spend my days making systems reliable. And I was presenting with the most fragile link in the room.

## My slides were already HTML

For about a year I have not made a slide by hand. I describe the content, a tool (or an assistant) gives me one clean HTML page per slide, with highlighted code and SVG diagrams. Producing slides stopped being the problem. Presenting them was: showing them on the big screen, moving forward without walking back to the laptop, having my notes in front of me, and letting the people at the back (or at home) follow on their own screen.

Nothing did all three without an account, a framework, or a service in the middle. So I wrote it.

## A folder, a binary, three screens

Deckhand takes a folder of HTML files (one per slide, plus a small `deck.json`) and gives you:

- **the stage**: the slide full screen, in any browser, so on any screen;
- **the remote**: your phone, with thumbnails, speaker notes, a timer and a laser pointer;
- **the audience**: a link people open on their own device, following the stage live.

```
deckhand validate talk/     # check the deck, list every problem
deckhand present  talk/     # stage + remote + audience on the LAN
deckhand push     talk/     # publish on a hub, get a permanent link
```

One Go binary for macOS, Linux and Windows. No account to present locally: the command prints two QR codes, you scan the remote with your phone, you open the stage on the screen, done. Everything is synchronised over WebSocket, fragments and laser included.

## Security first, because it is my job

Serving HTML you did not write is self-service XSS if you are not careful. Every slide runs in an `<iframe sandbox="allow-scripts">`, never `allow-same-origin`: a slide's script sees no cookies, no session, no other slide. App pages carry a strict CSP. On the hub, decks are served from a separate origin. Archives are checked on import: no path climbing out, allowed file types only, a size cap. Boring rules, in from day one.

## The hub, for those who want to share

Presenting on the LAN is enough for a room. For remote people, or to leave a link after the talk, there is [deckhand.show](/): push the deck, get a permanent link, let viewers follow from anywhere, see how many did. Magic-link sign-in, no password. The hub is also [self-hostable](/docs/HUB): one binary, one PostgreSQL, a `docker compose` in the repo.

## What I want to hear

This is a 1.0. I have only tested my own decks. If you present, for work, for a class, for a meetup, try it and tell me what breaks. That is exactly what I am after.

- Install: `brew install stranix79/tap/deckhand`, or the binaries in the [releases](https://github.com/stranix79/deckhand/releases).
- Code, MIT for the local mode: [github.com/stranix79/deckhand](https://github.com/stranix79/deckhand).
- The [newsletter](/#newsletter) if you want to hear about the next versions, one address and nothing else.
