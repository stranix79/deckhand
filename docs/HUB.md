# Hub

The hub is the same `deckhand` binary in `serve` mode: multi-user, backed by
PostgreSQL 16, with magic-link sign-in, deck uploads, hosted and relayed
presentations, permanent links, statistics and Stripe billing.

`internal/hub` is under the Business Source License 1.1 (`LICENSE.hub`):
you may self-host it for your own use; offering it as a paid service to
third parties needs a licence from CODE79 until the change date.

## Run it

```
cp .env.example .env            # fill the secrets
docker compose -f docker-compose.hub.yml up -d
curl -s http://localhost:8080/healthz
```

Put a reverse proxy (nginx, Caddy, Traefik) in front with TLS for **two**
hostnames: the app (`deckhand.example.com`) and the deck origin
(`decks.deckhand.example.com`). Both point to the same container; the hub
picks the role from the `Host` header. WebSockets must be proxied
(`Upgrade`/`Connection` headers) on the app hostname.

Without Docker: `deckhand serve --pg postgres://… --base-url https://… --deck-origin https://…`
with the environment below. Migrations run automatically at start.

## Environment

| Variable | Default | Meaning |
|---|---|---|
| `DECKHAND_ADDR` | `:8080` | Listen address. |
| `DECKHAND_PG` | required | PostgreSQL URL. |
| `DECKHAND_BASE_URL` | `http://localhost:8080` | Public URL of the app. Used in e-mails, QR codes, cookies (`Secure` when https). |
| `DECKHAND_DECK_ORIGIN` | same origin | Origin of the deck files (`https://decks.…`). Strongly recommended: slides are untrusted HTML. |
| `DECKHAND_DATA_DIR` | `./data` | Where uploaded decks are extracted. |
| `DECKHAND_SECRET` | required | ≥ 32 random characters. |
| `MAIL_HOST` `MAIL_PORT` `MAIL_USER` `MAIL_PASSWORD` `MAIL_FROM` | | SMTP for magic links. 465 = TLS, other ports = STARTTLS. |
| `DECKHAND_DEV_LOG_MAGIC_LINKS` | | `1` logs sign-in links instead of e-mailing them (development). |
| `DECKHAND_FREE_MAX_DECKS` | `1` | Free plan: decks per user. |
| `DECKHAND_FREE_MAX_VIEWERS` | `10` | Free plan: remote viewers per presentation (the 11th gets "room is full"). |
| `DECKHAND_FREE_LINK_DAYS` | `7` | Free plan: lifetime of `/d/{user}/{slug}` after the last upload. |
| `DECKHAND_MAX_DECK_MB` | `200` | Upload limit. |
| `DECKHAND_MAGIC_LINK_TTL` | `15m` | Sign-in link validity. |
| `DECKHAND_COOKIE_TTL` | `720h` | Browser session. |
| `DECKHAND_STRIPE_SECRET_KEY` `DECKHAND_STRIPE_WEBHOOK_SECRET` `DECKHAND_STRIPE_PRICE_ID` | | Billing. All three or nothing. |

## Routes

| Path | What |
|---|---|
| `/` | landing (`site/index.html`), `/docs/*`, `/changelog` |
| `/login`, `/auth/callback`, `/logout` | magic-link sign-in |
| `/app` | decks, upload, present now, live presentations, API token |
| `/d/{user}/{slug}` | permanent link (latest version, detached viewer, indexable) |
| `/s/{code}?t=` `/r/{code}?t=` `/v/{code}` | the three screens; on the hub the stage needs the token too |
| `/api/v1/me`, `/api/v1/decks`, `/api/v1/presentations` | CLI, `Authorization: Bearer <token>` |
| `/api/v1/relay/{code}?t=` | WebSocket from `deckhand present --hub` |
| `/billing`, `/webhooks/stripe` | Stripe Checkout, portal, webhook |
| `/healthz`, `/metrics` | health, Prometheus |

## Presenting through the hub

Two ways:

* **Hosted**: upload, click *Present now*. You get a stage link, a remote
  link and an audience link; everything runs on the hub. No CLI.
* **Relayed**: `deckhand present talk/ --hub https://deckhand.example.com`.
  The stage and the remote stay on your LAN (works even if the venue Wi-Fi is
  bad); the hub receives every state change and serves remote viewers. If the
  hub drops, the local presentation is unaffected.

After a presentation, `/app` → *stats*: unique viewers, peak audience,
audience per slide (SVG drawn server-side), time per slide.

## Stripe

Create a monthly recurring price, put its id in `DECKHAND_STRIPE_PRICE_ID`,
and a webhook endpoint `https://deckhand.example.com/webhooks/stripe` sending
`checkout.session.completed`, `customer.subscription.created`,
`customer.subscription.updated`, `customer.subscription.deleted`. Its signing
secret goes in `DECKHAND_STRIPE_WEBHOOK_SECRET`.

Plan resolution is one query: a user is *Pro* while a row in `subscriptions`
has status `active` or `trialing`.

## Metrics

`deckhand_sessions_active`, `deckhand_ws_connections`, `deckhand_viewers`,
`deckhand_relay_connections`, `deckhand_relay_apply_seconds` (histogram),
`deckhand_http_requests_total{status}`.

## Backups

`pg_dump` the database and copy `DECKHAND_DATA_DIR` (deck files). Decks are
stored as plain files under `decks/<deck id>/v<n>/`.
