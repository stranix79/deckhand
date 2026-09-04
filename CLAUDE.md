# Deckhand — working notes for every session

Read this before touching the repo. The full brief lives in docs/BRIEF.md.

## Non-negotiable decisions (from the brief)
- Go 1.23+, single module `github.com/stranix79/deckhand`, single binary `deckhand`, no CGO. Cross-compile darwin/arm64, darwin/amd64, linux/arm64, linux/amd64, windows/amd64.
- HTTP: stdlib `net/http` + `github.com/go-chi/chi/v5`. WebSocket: `github.com/coder/websocket`.
- Front (web/): HTML/CSS/JS vanilla, no framework, no build step, embedded with `embed`. Readable by a human with a text editor. Comment the JS for a sysadmin/DBA who is not a front-end dev.
- Local mode (`present`): no database, state in memory. Hub (`serve`): PostgreSQL 16 via `pgx/v5`, migrations with `golang-migrate`, embedded.
- Hub auth: magic link by e-mail, HttpOnly/Secure/SameSite=Lax cookie sessions. No OAuth. Payment: Stripe Checkout + webhooks, monthly subscription only.
- Slide security from day one: `<iframe sandbox="allow-scripts">` (never `allow-same-origin`), strict CSP on app pages, decks served from a distinct origin in Hub mode (`decks.<domain>` vs `app.<domain>`, env-configurable), deck max 200 MB / 500 slides, zip slip and path traversal refused, allowed file types only (.html .css .js .json images videos fonts .svg .pdf; .svg served as image/svg+xml + Content-Disposition attachment except for <img>).
- Tests: `go test ./...` green at every milestone. Unit tests on deck parsing and protocol; integration test starting the server with two WebSockets.
- Quality: gofmt, go vet, golangci-lint (config in .golangci.yml), zero warnings. Conventional commits (`feat:`, `fix:`, `docs:`…), messages in English.
- License: MIT everywhere except internal/hub (BSL 1.1, LICENSE.hub).
- Language: French in conversation, English in code, commits and docs. Landing is bilingual.
- Undecided points: pick the simplest option, log it in docs/DECISIONS.md with date and one line of why, keep going. Ask only if the milestone is blocked.
- Stop at the end of each milestone and report in 10 lines max: what exists, how to test it, what is open.

## Milestones
1. ✅ (2026-09-04) Deck & validation (internal/deck, `deckhand validate`, examples/ship-it) — DONE when `deckhand validate examples/ship-it` exits 0 and a zip with `../` exits 1 with a clear message.
2. ✅ (2026-09-04) Present local: server, WebSocket, stage + remote, ASCII QR codes, integration test.
3. ✅ (2026-09-04) Viewer + local polish, goreleaser snapshot, Homebrew tap. v0.1.0.
4. ✅ (2026-09-04) Hub: auth, decks, remote viewers, relay.
5. ✅ (2026-09-04) Hub: stats, Stripe, limits, metrics, deployment (deckhand.stranix.net on hawking).
6. ✅ (2026-09-04) Site and docs. v1.0.0.

## Makefile commands
- `make build`   → ./deckhand (current OS)
- `make test`    → go test ./... (race on)
- `make lint`    → gofmt check + go vet + golangci-lint
- `make run-local` → deckhand present examples/ship-it --open
- `make run-hub` → docker compose -f docker-compose.hub.yml up
- `make release` → goreleaser release --snapshot --clean
- `make validate` → deckhand validate examples/ship-it

## Layout
cmd/deckhand (cobra CLI) · internal/deck (format, parsing, validation, archives, natural sort) · internal/session (state, WS protocol, broadcast) · internal/local (present server, LAN IP, ASCII QR) · internal/hub (serve: auth, decks, stats, stripe, relay) · internal/ui (serves embedded web/) · web/ (stage, remote, viewer, shared) · site/ (landing + docs) · docs/ · examples/ship-it · migrations/

## Local development of the hub
- `make dev-hub` starts a throwaway PostgreSQL (`scripts/dev-pg.sh`, port 5499, needs Homebrew `postgresql@16`) and the hub on :8080 with magic links printed in the log.
- Deployment on hawking: `deploy/hawking/README.md` (compose + vhost also live in stranix-git).
