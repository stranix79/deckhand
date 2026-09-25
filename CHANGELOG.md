# Changelog

All notable changes to Deckhand are documented here. The format follows Keep a Changelog; versions follow SemVer.

## [Unreleased]

## [1.4.3] - 2026-09-25

### Changed
- The blog is now part of the site rather than of the app: it renders with the landing's header (logo, navigation, EN/FR switch that links to the translation), footer and fonts, in one text column. The index is a dated list of titles with a one-line summary; a post is a title, a date and the prose, followed by a next-post link. Post images are kept only for link previews (Open Graph). New stylesheet `/static/hub/site.css`.

## [1.4.2] - 2026-09-25

### Changed
- Blog redesigned to match the landing: the hub layout now loads the same display and serif fonts, the index opens with a headline and a featured latest post on a dark panel followed by cards, and the post page gets a large title, a full-width hero with credit, a readable column, dark code blocks and a "try Deckhand" block at the end.

## [1.4.1] - 2026-09-25

### Fixed
- The hub stylesheet (`/static/hub/hub.css`) was served empty by release builds: `web/hub` was missing from the embed directive, so every hub page (docs, comparisons, blog) rendered unstyled. Embedded now, with a test that checks the stylesheet has content.

## [1.4.0] - 2026-09-25

### Added
- A blog on the site, in English at `/blog` and in French at `/blog/fr`, with one Markdown file per post and per language embedded in the binary, `hreflang` pairs between translations, a per-post Open Graph image, RSS feeds (`/blog/feed.xml`, `/blog/fr/feed.xml`) and the posts in the sitemap. Three posts to start: the launch, the 1.3 release, and a call to try and share the project. Blog link in the landing navigation and the hub layout.

## [1.3.1] - 2026-09-23

### Fixed
- `/changelog` on the hub is now served by every build: the changelog is embedded from the repository root instead of being copied into `docs/` by the Makefile, which the CI, goreleaser and Homebrew builds never did (their binaries answered 404, and the release pipeline failed on that test).
- The indexability test matches the exact robots meta tag instead of the word "noindex", which the changelog text itself contains.

## [1.3.0] - 2026-09-23

### Added
- The fifteen second screen recording of the three screens on the landing page, served at `/static/site/demo.mp4` with Range support (Safari).
- `docs/LLM.md` and a README section with a ready-to-paste prompt that makes any language model produce a deck that passes `deckhand validate`; served at `/docs/LLM` on the hub.
- Comparison pages on the site: `/vs` and `/vs/reveal-js`, `/vs/slidev`, `/vs/google-slides`, with a table, where each tool is the better pick, and proper title, description, canonical and Open Graph tags. Hub pages only carry `noindex` when they do not declare that metadata.
- `/sitemap.xml` (landing, docs, changelog, comparisons) and `/robots.txt`; the docs and changelog pages now carry a description and canonical URL so they can be indexed.

## [1.2.1] - 2026-09-22

### Changed
- New Open Graph image for the landing (dark card with the terminal and the tagline) so link previews on LinkedIn, Facebook and chat apps stop showing a cropped screenshot of the page.

## [1.2.0] - 2026-09-22

### Added
- Newsletter sign-up on the landing page (`POST /newsletter`), subscribing through Listmonk's public API server to server, with built-in anti-spam: honeypot field, signed timestamp (refused under 3 s or after 2 h), 5 sign-ups per IP per hour, address validation. Configured with `DECKHAND_LISTMONK_URL`, `DECKHAND_NEWSLETTER_LIST_FR` and `DECKHAND_NEWSLETTER_LIST_EN`.

## [1.1.2] - 2026-09-21

### Added
- Optional Google Analytics 4 tag on the landing page (`DECKHAND_ANALYTICS_ID`); app pages never load it.

## [1.1.1] - 2026-09-08

### Changed
- Public domain is now deckhand.show (deck origin decks.deckhand.show); deckhand.stranix.net redirects.

## [1.1.0] - 2026-09-06

### Added
- Billing through Odoo Subscriptions (`DECKHAND_BILLING=odoo`): the hub links to the shop product, syncs in-progress subscriptions over XML-RPC and matches them by e-mail. Stripe remains available (`DECKHAND_BILLING=stripe`).

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
