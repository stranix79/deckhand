// DeckhandSocket keeps one WebSocket open to the session and reconnects by
// itself. Screens never touch the socket directly: they give callbacks.
//
//   const sock = new DeckhandSocket({
//     code: 'K7RTQP', role: 'stage', token: null,
//     onFrame:  function (frame) { ... },   // one parsed JSON object per message
//     onStatus: function (status) { ... }, // 'connecting' | 'open' | 'closed' | 'forbidden' | 'gone'
//   });
//   sock.send({ op: 'next' });
//
// The server sends the full deck and the full state on every (re)connection,
// so a screen only has to render what it receives: no local catch-up logic.
'use strict';

class DeckhandSocket {
  constructor(opts) {
    this.code = opts.code;
    this.role = opts.role;
    this.token = opts.token || null;
    this.onFrame = opts.onFrame || function () {};
    this.onStatus = opts.onStatus || function () {};
    this.delay = 500;          // reconnection backoff, doubles up to 8 s
    this.ws = null;
    this.closedByUs = false;
    this.connect();
  }

  url() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    let u = proto + '://' + location.host + '/ws/' + encodeURIComponent(this.code) +
      '?role=' + this.role;
    if (this.token) u += '&t=' + encodeURIComponent(this.token);
    return u;
  }

  connect() {
    this.onStatus('connecting');
    const ws = new WebSocket(this.url());
    this.ws = ws;
    ws.onopen = () => { this.delay = 500; this.onStatus('open'); };
    ws.onmessage = (e) => {
      let frame;
      try { frame = JSON.parse(e.data); } catch (err) { return; }
      this.onFrame(frame);
    };
    ws.onclose = (e) => {
      if (this.closedByUs) return;
      // 1008 = policy violation: the server refused the role/token, or the
      // session is gone. Reconnecting would not help; tell the screen.
      if (e.code === 1008 || e.code === 4403) { this.onStatus('forbidden'); return; }
      if (e.code === 4404) { this.onStatus('gone'); return; }
      if (e.code === 4429) { this.onStatus('full'); return; }
      this.onStatus('closed');
      setTimeout(() => this.connect(), this.delay);
      this.delay = Math.min(this.delay * 2, 8000);
    };
    ws.onerror = () => { /* onclose follows, nothing to do here */ };
  }

  send(obj) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(obj));
      return true;
    }
    return false;
  }

  close() {
    this.closedByUs = true;
    if (this.ws) this.ws.close();
  }
}

// Reads the session code from the page URL: /s/K7RTQP, /r/K7RTQP, /v/K7RTQP.
function sessionCodeFromPath() {
  const m = location.pathname.match(/^\/(?:s|r|v)\/([A-Z0-9]+)/i);
  return m ? m[1].toUpperCase() : '';
}

// Reads the remote token from ?t=…
function tokenFromQuery() {
  return new URLSearchParams(location.search).get('t');
}
