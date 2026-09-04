# Changelog

All notable changes to Deckhand are documented here. The format follows Keep a Changelog; versions follow SemVer.

## [Unreleased]

## [1.0.5] - 2026-09-04

### Fixed
- Screens' JS/CSS were cached for an hour by browsers, so an upgraded binary could still run the previous stage script. They are now revalidated on every load (ETag = version).

## [1.0.4] - 2026-09-04

### Fixed
- Stage: entrance animations of the next slide played while it was preloaded off screen, so going forward showed them already finished (going back played them). The hidden frame now only warms the cache; the visible frame loads the slide when it is shown.

## [1.0.3] - 2026-09-04

### Fixed
- Stage: the audience QR could stay on screen much longer than 15 s when the stage tab was in the background (browsers throttle timers); it now hides on a deadline checked on focus and on every frame. The remote's button toggles Show/Hide.

## [1.0.2] - 2026-09-04

### Added
- `examples/keynote` ("Field notes"): a 17-slide showcase with photos, live code, an interactive chart, a git diff and an HTML-vs-PPTX scorecard.

## [1.0.1] - 2026-09-04

### Added
- Examples page (`/docs/EXAMPLES`) and a link from the landing to the live example deck and its source.

## [1.0.0] - 2026-09-04

The hub, the site and the docs.

### Added
- `deckhand serve`: multi-user hub on PostgreSQL 16 (embedded migrations), magic-link
  sign-in, deck uploads (web and `deckhand push`), hosted presentations ("Present now"),
  relayed presentations (`deckhand present --hub`), permanent links `/d/{user}/{slug}`,
  API tokens, `deckhand login`.
- Free plan limits (1 deck, 10 remote viewers, 7-day links) from the environment; Stripe
  Checkout, customer portal and webhooks for the Pro plan.
- Statistics per presentation: unique viewers, peak, audience per slide (server-side SVG),
  time per slide. Prometheus metrics on `/metrics`, `/healthz`.
- Deck files served from a separate origin (`DECKHAND_DECK_ORIGIN`); stage needs the token on the hub.
- Landing page, rendered docs (`/docs/*`), `/changelog`, Open Graph tags.
- Dockerfile (distroless), `docker-compose.hub.yml`, `docs/HUB.md`, `docs/SECURITY.md`.

### Fixed
- Default title of an archive deck is the archive name, not the temporary directory.

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
