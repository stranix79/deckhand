// Viewer screen: the audience.
//
// Two modes, decided locally (the server does not know):
//   live      – the screen follows the "state" frames from the server;
//   detached  – the person swiped or pressed an arrow and browses alone;
//               a "back to live" button re-aligns on the last state.
// Public notes (deck.json "public": true) are shown under the slide.
'use strict';

(function () {
  applyI18n();

  const code = sessionCodeFromPath();
  const $ = (id) => document.getElementById(id);
  const box = $('frame'), laser = $('laser'), notesWrap = $('notes-wrap'), notes = $('notes');
  const mode = $('mode'), modeText = $('mode-text'), counter = $('counter'), viewers = $('viewers');
  const back = $('back'), banner = $('banner'), hint = $('hint');

  let deck = null, state = null, frame = null;
  let detached = false, local = 0; // local = slide shown while detached

  function shownSlide() { return detached ? local : (state ? state.slide : 0); }

  function render() {
    if (!deck) return;
    const n = shownSlide(), last = deck.slides.length - 1;
    frame.show(n);
    frame.setFragment(detached ? 0 : (state ? state.fragment || 0 : 0));
    counter.textContent = (n + 1) + ' / ' + (last + 1);
    const s = deck.slides[n];
    const hasNotes = !!(s && s.public && s.notes);
    notesWrap.hidden = !hasNotes;
    notes.textContent = hasNotes ? s.notes : '';
    placeLaser(laser, frame, detached ? null : (state && state.pointer));
    mode.className = 'pill ' + (detached ? 'warn' : 'live');
    modeText.textContent = detached ? t('detached') : t('live');
    back.hidden = !detached;
    frame.fit();
  }

  function detach(delta) {
    if (!deck) return;
    if (!detached) { detached = true; local = state ? state.slide : 0; }
    local = Math.min(deck.slides.length - 1, Math.max(0, local + delta));
    render();
  }
  back.addEventListener('click', () => { detached = false; render(); });

  // Keyboard and swipe navigation detach the viewer.
  document.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowRight' || e.key === ' ') { e.preventDefault(); detach(+1); }
    if (e.key === 'ArrowLeft') { e.preventDefault(); detach(-1); }
    if (e.key === 'Escape' || e.key === 'l') { detached = false; render(); }
  });
  let touchX = null, touchY = null;
  box.addEventListener('touchstart', (e) => { touchX = e.touches[0].clientX; touchY = e.touches[0].clientY; }, { passive: true });
  box.addEventListener('touchend', (e) => {
    if (touchX === null) return;
    const dx = e.changedTouches[0].clientX - touchX, dy = e.changedTouches[0].clientY - touchY;
    touchX = null;
    if (Math.abs(dx) > 40 && Math.abs(dx) > Math.abs(dy) * 1.5) detach(dx < 0 ? +1 : -1);
  }, { passive: true });

  window.addEventListener('resize', () => { if (frame) { frame.fit(); render(); } });
  setTimeout(() => hint.classList.add('gone'), 6000);

  const sock = new DeckhandSocket({
    code: code, role: 'viewer', token: null,
    onFrame: function (f) {
      switch (f.op) {
        case 'deck':
          deck = f.deck;
          $('title').textContent = deck.title;
          document.title = deck.title;
          if (!frame) { frame = new SlideFrame(box, deck); box.appendChild(laser); }
          render();
          break;
        case 'state':
          state = f.state;
          render();
          break;
        case 'viewers':
          viewers.textContent = f.count + ' ' + (f.count === 1 ? t('viewer1') : t('viewers'));
          break;
      }
    },
    onStatus: function (s) {
      banner.hidden = true;
      if (s === 'gone') { banner.textContent = t('notFound'); banner.hidden = false; }
      else if (s === 'closed') { mode.className = 'pill off'; modeText.textContent = t('reconnecting'); }
    },
  });
})();
