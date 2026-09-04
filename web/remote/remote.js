// Remote screen: the presenter's phone.
//
// It shows the current and next slide, the notes, a timer, the viewer count,
// and sends ops to the server: next/prev, laser pointer, black screen, QR.
// It never decides anything itself: it renders the "state" frames it gets.
'use strict';

(function () {
  applyI18n();

  const code = sessionCodeFromPath();
  const token = tokenFromQuery();
  const $ = (id) => document.getElementById(id);
  const cur = $('cur'), nxt = $('nxt'), laser = $('laser');
  const notes = $('notes'), counter = $('counter'), timer = $('timer'), viewers = $('viewers');
  const conn = $('conn'), connText = $('conn-text'), banner = $('banner'), end = $('end');
  const laserBtn = $('laser-btn'), blackBtn = $('black-btn'), qrBtn = $('qr-btn'), padHint = $('pad-hint');

  let deck = null, state = null, curFrame = null, nxtFrame = null;
  let laserMode = false;

  // "Add to home screen": point the manifest at a URL that keeps the token.
  if (token) {
    $('manifest').href = '/manifest/' + encodeURIComponent(code) + '.json?t=' + encodeURIComponent(token);
  }

  // --- rendering -------------------------------------------------------------

  function render() {
    if (!deck || !state) return;
    const n = state.slide, last = deck.slides.length - 1;
    curFrame.show(n);
    curFrame.setFragment(state.fragment || 0);
    if (n < last) { nxtFrame.show(n + 1); end.hidden = true; nxtFrame.iframe.style.visibility = 'visible'; }
    else { nxtFrame.show(-1); nxtFrame.iframe.style.visibility = 'hidden'; end.hidden = false; }
    counter.textContent = (n + 1) + ' / ' + (last + 1);
    const s = deck.slides[n];
    notes.textContent = (s && s.notes) ? s.notes : t('noNotes');
    notes.classList.toggle('empty', !(s && s.notes));
    blackBtn.classList.toggle('on', !!state.black);
    placeLaser(laser, curFrame, state.pointer);
    renderTimer();
  }

  // The timer starts on the first "next" (state.startedAt is set by the
  // server) and is computed from that timestamp, so it survives reloads.
  function renderTimer() {
    if (!state || !state.startedAt) { timer.textContent = '0:00'; timer.classList.add('idle'); timer.title = t('timerIdle'); return; }
    const sec = (Date.now() - new Date(state.startedAt).getTime()) / 1000;
    timer.textContent = formatClock(sec);
    timer.classList.remove('idle');
  }
  setInterval(renderTimer, 1000);
  timer.addEventListener('click', () => sock.send({ op: 'reset' }));

  function fitAll() { if (curFrame) curFrame.fit(); if (nxtFrame) nxtFrame.fit(); if (state) placeLaser(laser, curFrame, state.pointer); }
  window.addEventListener('resize', fitAll);

  // Haptic tick on slide change, when the phone supports it.
  let lastSlide = -1;
  function haptic() { if (navigator.vibrate) navigator.vibrate(12); }

  // --- socket ----------------------------------------------------------------------

  function setConn(kind, text) { conn.className = 'pill ' + kind; connText.textContent = text; }

  const sock = new DeckhandSocket({
    code: code, role: 'remote', token: token,
    onFrame: function (f) {
      switch (f.op) {
        case 'deck':
          deck = f.deck;
          $('title').textContent = deck.title;
          document.title = deck.title + ' · ' + t('remote');
          if (!curFrame) {
            curFrame = new SlideFrame(cur, deck);
            nxtFrame = new SlideFrame(nxt, deck);
            cur.appendChild(laser);
            fitAll();
          }
          break;
        case 'state':
          state = f.state;
          if (lastSlide !== -1 && lastSlide !== state.slide) haptic();
          lastSlide = state.slide;
          render();
          break;
        case 'viewers':
          viewers.textContent = f.count + ' ' + (f.count === 1 ? t('viewer1') : t('viewers'));
          break;
      }
    },
    onStatus: function (s) {
      banner.hidden = true;
      if (s === 'open') setConn('live', t('live'));
      else if (s === 'connecting') setConn('warn', t('connecting'));
      else if (s === 'closed') setConn('off', t('reconnecting'));
      else if (s === 'forbidden') { setConn('off', t('offline')); banner.textContent = t('forbidden'); banner.hidden = false; }
      else if (s === 'gone') { setConn('off', t('offline')); banner.textContent = t('notFound'); banner.hidden = false; }
    },
  });

  // --- buttons -----------------------------------------------------------------------

  $('next').addEventListener('click', () => sock.send({ op: 'next' }));
  $('prev').addEventListener('click', () => sock.send({ op: 'prev' }));
  blackBtn.addEventListener('click', () => sock.send({ op: 'black' }));
  qrBtn.addEventListener('click', () => sock.send({ op: 'qr' }));
  laserBtn.addEventListener('click', () => {
    laserMode = !laserMode;
    laserBtn.classList.toggle('on', laserMode);
    cur.classList.toggle('pad', laserMode);
    padHint.hidden = !laserMode;
    if (!laserMode) sock.send({ op: 'pointer', x: null });
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowRight' || e.key === ' ') sock.send({ op: 'next' });
    if (e.key === 'ArrowLeft') sock.send({ op: 'prev' });
  });

  // --- laser touchpad ------------------------------------------------------------------
  // In laser mode the current thumbnail is a touchpad: the finger position,
  // normalised to the slide area, becomes the pointer on the stage. Sent at
  // most every 33 ms; lifting the finger clears the pointer.
  let lastSent = 0;
  function padPointer(e) {
    if (!laserMode || !curFrame || !curFrame.offset) return;
    e.preventDefault();
    const r = cur.getBoundingClientRect();
    const p = e.touches ? e.touches[0] : e;
    const x = (p.clientX - r.left - curFrame.offset.x) / curFrame.offset.w;
    const y = (p.clientY - r.top - curFrame.offset.y) / curFrame.offset.h;
    const now = Date.now();
    if (now - lastSent < 33) return;
    lastSent = now;
    sock.send({ op: 'pointer', x: Math.min(1, Math.max(0, x)), y: Math.min(1, Math.max(0, y)) });
  }
  function padEnd(e) { if (laserMode) { e.preventDefault(); sock.send({ op: 'pointer', x: null }); } }
  cur.addEventListener('touchstart', padPointer, { passive: false });
  cur.addEventListener('touchmove', padPointer, { passive: false });
  cur.addEventListener('touchend', padEnd, { passive: false });
  cur.addEventListener('touchcancel', padEnd, { passive: false });
  cur.addEventListener('mousedown', (e) => { if (laserMode) { padPointer(e); cur.dataset.drag = '1'; } });
  cur.addEventListener('mousemove', (e) => { if (cur.dataset.drag) padPointer(e); });
  window.addEventListener('mouseup', (e) => { if (cur.dataset.drag) { delete cur.dataset.drag; padEnd(e); } });
})();
