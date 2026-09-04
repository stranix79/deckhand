// Stage screen: the projector.
//
// What it does:
//   1. connects to the session as "stage";
//   2. shows the current slide, scaled to the screen, and preloads the next;
//   3. draws the laser where the remote says;
//   4. answers the server's fragment question ("ask") by asking the slide;
//   5. reacts to keys (→ ← Space PageUp PageDown Home End, F, B) by sending
//      ops to the server — the server is the only source of truth, the stage
//      never changes slide on its own.
'use strict';

(function () {
  applyI18n();

  const code = sessionCodeFromPath();
  const token = tokenFromQuery(); // only needed on the hub (StageNeedsToken)
  const $ = (id) => document.getElementById(id);
  const box = $('frame'), laser = $('laser'), black = $('black');
  const qr = $('qr'), qrImg = $('qr-img'), qrUrl = $('qr-url');
  const status = $('status'), statusText = $('status-text'), hint = $('hint');

  let deck = null;       // from the "deck" frame
  let state = null;      // from the last "state" frame
  let main = null;       // SlideFrame on screen
  let preload = null;    // SlideFrame loading the next slide, off screen
  let qrTimer = null, lostTimer = null, hintTimer = null, cursorTimer = null;

  // --- rendering -------------------------------------------------------------

  function render() {
    if (!deck || !state) return;
    const n = state.slide;
    if (main.index !== n) {
      // If the preload frame already holds slide n, swap the two frames:
      // the slide appears instantly, without a white flash.
      if (preload.index === n) {
        const tmp = main; main = preload; preload = tmp;
        main.iframe.style.visibility = 'visible';
        preload.iframe.style.visibility = 'hidden';
      } else {
        main.show(n);
      }
      preload.show(Math.min(n + 1, deck.slides.length - 1));
      if (preload.index === n) preload.show(-1); // last slide: nothing to preload
    }
    main.setFragment(state.fragment || 0);
    placeLaser(laser, main, state.pointer);
    black.hidden = !state.black;
  }

  function fitAll() {
    if (main) main.fit();
    if (preload) preload.fit();
    if (state) placeLaser(laser, main, state.pointer);
  }
  window.addEventListener('resize', fitAll);

  // --- fragment question -------------------------------------------------------

  // The server asks "would the current slide handle next/prev?". We forward
  // the question to the slide and answer within 150 ms either way.
  async function onAsk(frame) {
    const handled = await main.ask(frame.dir);
    sock.send({ op: 'answer', seq: frame.seq, handled: handled });
  }

  // --- overlays ------------------------------------------------------------------

  // The QR overlay hides at a deadline (qrUntil), not only on a timer:
  // browsers throttle timers of background tabs to once a minute, so a
  // timer alone can leave the QR on screen far longer than 15 s. The
  // deadline is re-checked whenever the tab becomes visible and on every
  // server frame.
  let qrUntil = 0;
  function showQr(seconds) {
    if (seconds === 0) { hideQr(); return; }
    qrImg.src = '/qr/' + encodeURIComponent(code) + '/viewer.png?s=' + Date.now();
    qr.hidden = false;
    qrUntil = Date.now() + (seconds || 15) * 1000;
    clearTimeout(qrTimer);
    qrTimer = setTimeout(checkQr, (seconds || 15) * 1000 + 50);
  }
  function hideQr() { qr.hidden = true; qrUntil = 0; clearTimeout(qrTimer); }
  function checkQr() { if (qrUntil && Date.now() >= qrUntil) hideQr(); }
  document.addEventListener('visibilitychange', checkQr);
  window.addEventListener('focus', checkQr);

  function setStatus(kind, text) {
    if (!kind) { status.hidden = true; return; }
    status.className = 'pill status ' + kind;
    statusText.textContent = text;
    status.hidden = false;
  }

  // --- socket ----------------------------------------------------------------------

  const sock = new DeckhandSocket({
    code: code, role: 'stage', token: token,
    onFrame: function (f) {
      switch (f.op) {
        case 'deck':
          deck = f.deck;
          qrUrl.textContent = f.viewerUrl || '';
          if (!main) {
            main = new SlideFrame(box, deck);
            preload = new SlideFrame(box, deck);
            preload.iframe.style.visibility = 'hidden';
            box.appendChild(laser); // keep the laser above both iframes
          }
          break;
        case 'state':
          state = f.state;
          render();
          checkQr();
          break;
        case 'ask':
          onAsk(f);
          break;
        case 'qr':
          showQr(f.seconds);
          break;
      }
    },
    onStatus: function (s) {
      clearTimeout(lostTimer);
      if (s === 'open') { setStatus(null); return; }
      if (s === 'forbidden') { setStatus('off', t('forbidden')); return; }
      if (s === 'gone') { setStatus('off', t('notFound')); return; }
      // The brief: tolerate 30 s of outage without changing anything on
      // screen. After that, a small pill says we are reconnecting.
      lostTimer = setTimeout(() => setStatus('warn', t('reconnecting')), 30000);
    },
  });

  // --- keyboard and mouse ------------------------------------------------------------

  const last = () => (deck ? deck.slides.length - 1 : 0);
  document.addEventListener('keydown', (e) => {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    switch (e.key) {
      case 'ArrowRight': case ' ': case 'PageDown': case 'Enter':
        e.preventDefault(); sock.send({ op: 'next' }); break;
      case 'ArrowLeft': case 'PageUp': case 'Backspace':
        e.preventDefault(); sock.send({ op: 'prev' }); break;
      case 'Home': sock.send({ op: 'goto', slide: 0 }); break;
      case 'End': sock.send({ op: 'goto', slide: last() }); break;
      case 'b': case 'B': case '.': sock.send({ op: 'black' }); break;
      case 'q': case 'Q': showQr(15); break;
      case 'f': case 'F': toggleFullscreen(); break;
      case 'Escape': hideQr(); break;
    }
  });

  function toggleFullscreen() {
    if (document.fullscreenElement) { document.exitFullscreen(); return; }
    const el = document.documentElement;
    (el.requestFullscreen || el.webkitRequestFullscreen || function () {}).call(el);
  }
  // A click anywhere goes fullscreen (the brief), except on the QR overlay
  // which just closes.
  document.addEventListener('click', (e) => {
    if (!qr.hidden) { hideQr(); return; }
    if (!document.fullscreenElement) toggleFullscreen();
  });

  // Hide the hint after 6 s and the mouse cursor after 3 s of stillness.
  hintTimer = setTimeout(() => hint.classList.add('gone'), 6000);
  document.addEventListener('mousemove', () => {
    document.body.classList.remove('hide-cursor');
    clearTimeout(cursorTimer);
    cursorTimer = setTimeout(() => document.body.classList.add('hide-cursor'), 3000);
  });
})();
