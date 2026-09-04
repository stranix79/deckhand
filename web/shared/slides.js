// SlideFrame: one slide, in a sandboxed iframe, scaled to fit a box.
//
// Every slide is a full HTML document designed at deck.width × deck.height
// (1920×1080 by default). We never resize the document itself: the iframe
// keeps the design size and is scaled with CSS transform so the slide looks
// exactly the same on a 4K projector and on a phone thumbnail.
//
// The iframe is sandboxed with allow-scripts only (never allow-same-origin):
// the slide can run its JS but cannot read our cookies, our DOM or our
// storage, and cannot navigate the parent page.
//
// Fragments: a slide may implement the optional postMessage protocol
// (docs/FORMAT.md). SlideFrame.ask('next') asks the slide whether it handled
// the step; SlideFrame.setFragment(n) replays steps after a load so that a
// thumbnail or a late viewer shows the same fragment as the stage.
'use strict';

class SlideFrame {
  // box: the element that keeps the ratio (class "frame"); deck: the deck
  // object from the "deck" frame ({width, height, slides:[{url}]}).
  constructor(box, deck) {
    this.box = box;
    this.deck = deck;
    this.index = -1;         // slide currently loaded, -1 = none
    this.fragment = 0;       // fragment currently shown
    this.loaded = false;
    this.pendingFragment = 0;
    this.iframe = document.createElement('iframe');
    this.iframe.setAttribute('sandbox', 'allow-scripts');
    this.iframe.setAttribute('referrerpolicy', 'no-referrer');
    this.iframe.width = deck.width;
    this.iframe.height = deck.height;
    this.iframe.title = 'slide';
    this.iframe.addEventListener('load', () => {
      this.loaded = true;
      // Replay the fragments the stage is already showing.
      if (this.pendingFragment > 0) this.replay(this.pendingFragment);
    });
    box.appendChild(this.iframe);
    this.waiters = [];
    // Answers from the slide arrive as window messages; only trust the ones
    // coming from our own iframe.
    window.addEventListener('message', (e) => {
      if (e.source !== this.iframe.contentWindow) return;
      const m = e.data;
      if (!m || m.type !== 'deckhand:handled') return;
      const w = this.waiters.shift();
      if (w) w(!!m.handled);
    });
    this.fit();
  }

  // Scale the iframe so the whole slide fits the box, centred (letterbox).
  fit() {
    const bw = this.box.clientWidth, bh = this.box.clientHeight;
    if (!bw || !bh) return;
    const k = Math.min(bw / this.deck.width, bh / this.deck.height);
    const w = this.deck.width * k, h = this.deck.height * k;
    this.iframe.style.transform = 'scale(' + k + ')';
    this.iframe.style.left = ((bw - w) / 2) + 'px';
    this.iframe.style.top = ((bh - h) / 2) + 'px';
    this.scale = k;
    this.offset = { x: (bw - w) / 2, y: (bh - h) / 2, w: w, h: h };
  }

  // Load slide n (no-op if already there). Fragment is reset to 0.
  show(n) {
    if (n === this.index) return;
    const s = this.deck.slides[n];
    this.index = n;
    this.fragment = 0;
    this.pendingFragment = 0;
    this.loaded = false;
    this.waiters = [];
    this.iframe.src = s ? s.url : 'about:blank';
  }

  // Ask the slide to move one fragment. Resolves true if the slide handled
  // it (stay on this slide), false otherwise or after 150 ms of silence.
  ask(dir) {
    return new Promise((resolve) => {
      if (!this.loaded || !this.iframe.contentWindow) { resolve(false); return; }
      let done = false;
      const finish = (v) => { if (!done) { done = true; resolve(v); } };
      this.waiters.push(finish);
      this.iframe.contentWindow.postMessage({ type: 'deckhand:' + dir }, '*');
      setTimeout(() => {
        const i = this.waiters.indexOf(finish);
        if (i >= 0) this.waiters.splice(i, 1);
        finish(false);
      }, 150);
    });
  }

  // Bring the slide to fragment n by sending next/prev steps. Used by every
  // screen that mirrors the stage (thumbnails, viewers).
  setFragment(n) {
    if (!this.loaded) { this.pendingFragment = n; return; }
    this.replay(n);
  }

  replay(n) {
    const win = this.iframe.contentWindow;
    if (!win) return;
    while (this.fragment < n) { win.postMessage({ type: 'deckhand:next' }, '*'); this.fragment++; }
    while (this.fragment > n) { win.postMessage({ type: 'deckhand:prev' }, '*'); this.fragment--; }
  }
}

// Places a laser element at normalised (x, y) inside a SlideFrame's box.
function placeLaser(el, frame, pointer) {
  if (!pointer || !frame.offset) { el.classList.remove('on'); return; }
  el.style.left = (frame.offset.x + pointer.x * frame.offset.w) + 'px';
  el.style.top = (frame.offset.y + pointer.y * frame.offset.h) + 'px';
  el.classList.add('on');
}

// Formats seconds as m:ss or h:mm:ss.
function formatClock(sec) {
  sec = Math.max(0, Math.floor(sec));
  const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = sec % 60;
  const mm = (h ? String(m).padStart(2, '0') : String(m)), ss = String(s).padStart(2, '0');
  return (h ? h + ':' : '') + mm + ':' + ss;
}
