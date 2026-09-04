# Session protocol

A **session** is one loaded deck plus a state:

```json
{ "slide": 3, "fragment": 1, "pointer": { "x": 0.42, "y": 0.73 }, "startedAt": "2026-09-04T13:00:00Z", "black": false }
```

* `slide` – 0-based index in the deck.
* `fragment` – step inside the slide (0 = nothing revealed); only meaningful
  for slides that implement the fragment protocol (see [FORMAT.md](FORMAT.md)).
* `pointer` – laser position, normalised to the slide (0..1), or `null`.
* `startedAt` – set by the first `next`; drives the remote's timer. `null` until then.
* `black` – the stage shows a black screen.

A session is identified by a **code** of 6 characters from
`ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (no I, O, 0, 1: it is read aloud).
The **token** is a 32-character hex secret, generated with the session and
embedded in the remote's QR code.

## WebSocket `/ws/{code}?role=stage|remote|viewer[&t=token]`

One endpoint, one JSON object per text frame. The role is chosen at
connection time:

| Role | Token | May send | Receives notes |
|---|---|---|---|
| `remote` | required | everything | all |
| `stage` | optional locally, required on the hub | everything | all |
| `viewer` | – | nothing (sending closes the connection) | only `public: true` |

### Server → client

```
{op:"deck", deck:{title, ratio, width, height, slides:[{index, url, notes?, public?}]}, code, viewerUrl?}
{op:"state", state:{...}}          on connection and on every change
{op:"viewers", count:17}           on connection, then every 2 s when it changed
{op:"ask", seq:12, dir:"next"|"prev"}   to the stage only, see fragments
{op:"qr", seconds:15}              the stage shows the audience QR (0 = hide); sent to all
```

`viewerUrl` (the public link) is sent to the stage and the remote only.
Slide `url`s are relative to the app origin locally, absolute on the hub
(decks are served from a separate origin).

### Client → server

```
{op:"next"} | {op:"prev"} | {op:"goto", slide:3}
{op:"pointer", x:0.42, y:0.73} | {op:"pointer", x:null}
{op:"black"}                       toggle
{op:"qr", seconds?:15}            show the audience QR on the stage (seconds:0 hides it)
{op:"reset"}                       clear startedAt (timer)
{op:"answer", seq:12, handled:true|false}   stage only, see fragments
{op:"fragment", fragment:2}        stage only: report a fragment moved locally
```

Unknown ops are ignored and logged at debug level. `goto` clamps to the deck.

### Fragments: who decides?

The server does not know whether a slide has internal steps. So when a
`next`/`prev` arrives **and a stage is connected**, the server asks the
stage first:

```
remote ─next─▶ server ─ask{seq}─▶ stage ─postMessage deckhand:next─▶ slide
slide ─deckhand:handled{handled}─▶ stage ─answer{seq,handled}─▶ server
```

* `handled:true` → `fragment` ± 1, same slide, broadcast.
* `handled:false`, or no answer within 400 ms (150 ms in the slide + network),
  or the stage disconnects → slide ± 1, `fragment` reset to 0, broadcast.
* A `goto` never asks: it jumps and resets the fragment.

Without a stage (a viewer-only rehearsal) the slide changes immediately.

Viewers and the remote's thumbnails mirror the fragment by replaying
`deckhand:next` messages into their own iframe after it loads.

### Reconnection

Clients reconnect with exponential backoff (0.5 s → 8 s). Every connection
gets the full `deck` and `state`, so there is nothing to replay. The stage
tolerates 30 s of outage without changing anything on screen. The viewer can
**detach** (browse alone) purely client-side; the server never knows.

## HTTP endpoints shared by local and hub

| Path | Purpose |
|---|---|
| `/s/{code}` `/r/{code}?t=…` `/v/{code}` | the three screens |
| `/ws/{code}` | the WebSocket above |
| `/deck/{code}/{file}` | deck files (separate origin on the hub) |
| `/qr/{code}/viewer.png` | QR of the public link |
| `/manifest/{code}.json?t=…` | PWA manifest of the remote (keeps the token in `start_url`) |
| `/static/…` | embedded assets |

Deck files are served with the content type of their extension only
(anything else is 404), `X-Content-Type-Options: nosniff`, and for HTML a
`Content-Security-Policy: sandbox allow-scripts`. SVG gets
`Content-Disposition: attachment` unless the browser fetched it for an
`<img>` (`Sec-Fetch-Dest: image`).

App pages carry a strict CSP: `default-src 'none'`, same-origin scripts and
styles, no inline script, `frame-src` limited to the deck origin,
`frame-ancestors 'none'`.
