# Changelog

All notable changes to Deckhand are documented here. The format follows Keep a Changelog; versions follow SemVer.

## [Unreleased]

## [0.1.0] - 2026-09-04

Local mode: validate and present a deck on your LAN.

### Added
- Deck format: directory, `.zip` or `.tar.gz`, optional `deck.json`, natural ordering of `*.html` slides.
- `deckhand validate <deck>`: loads a deck, reports every problem, exits 1 on error.
- Example deck `examples/ship-it` (8 slides, notes, one slide with fragments).
- `deckhand present <deck>`: local server on the LAN, stage + remote + viewer screens,
  QR codes in the terminal, `--open`, `--port`, `--ip`, `--no-lan`.
- Session engine: WebSocket `/ws/{code}` with roles stage/remote/viewer, remote
  token, fragment negotiation with the stage, laser pointer, black screen,
  viewer count, audience QR on the stage.
- Stage: scaled sandboxed iframe, next-slide preload, keyboard control, fullscreen.
- Remote: current/next thumbnails, notes, timer, laser touchpad, haptics, PWA manifest.
- Viewer: live/detached modes, swipe, public notes.
