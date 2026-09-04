# Security

Slides are **untrusted HTML** written by users (or their AI). Deckhand treats
them as such from day one.

## Isolation of slides

* Every slide is rendered in `<iframe sandbox="allow-scripts">`. Never
  `allow-same-origin`: the slide has an opaque origin, cannot read the app's
  cookies, storage or DOM, cannot navigate the parent, cannot open popups.
* Deck files are served with `Content-Security-Policy: sandbox allow-scripts`
  as well, so a slide opened directly is sandboxed too.
* On the hub, deck files come from a **separate origin**
  (`DECKHAND_DECK_ORIGIN`), so even a sandbox escape would land on an origin
  without cookies.
* SVG files are sent with `Content-Disposition: attachment` unless requested
  by an `<img>`: an SVG opened as a document can run scripts.
* Only a closed list of file types is served (see FORMAT.md); everything else
  is a 404 even if present on disk. `X-Content-Type-Options: nosniff` everywhere.

## App pages

* Strict CSP: `default-src 'none'`, same-origin scripts and styles, no inline
  script on the three screens, `frame-src` limited to the deck origin,
  `frame-ancestors 'self'`.
* Cookies: `HttpOnly`, `Secure` (https), `SameSite=Lax`. Cookie-authenticated
  POSTs also require a same-origin `Origin`/`Referer`.
* Magic links: random 256-bit token, hashed at rest, 15 minutes, single use.
* API tokens: random 256-bit, hashed at rest, revocable by deleting the row.

## Session protocol

* The remote (and the stage on the hub) must present the session token,
  compared in constant time. Viewers may not send anything: a viewer that
  sends an op is disconnected.
* Session codes are 6 characters from a 32-symbol alphabet (~10⁹ codes),
  generated with `crypto/rand`.

## Uploads

* Archives are extracted with path checks (`..`, absolute paths, backslashes,
  drive letters refused), symlinks refused, uncompressed size and file count
  capped **during** extraction (zip bombs stop early).
* 200 MB and 500 slides per deck; `deck.json` is decoded strictly.

## Reporting

Write to security@stranix.net. Please do not open a public issue for a
vulnerability.
