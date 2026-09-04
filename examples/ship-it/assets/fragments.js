// Optional slide ↔ Deckhand protocol, fragment edition.
//
// Deckhand asks the slide before changing it:
//   {type:"deckhand:next"} / {type:"deckhand:prev"}
// The slide answers {type:"deckhand:handled", handled:true|false}. When it
// answers false (or nothing within 150 ms) Deckhand moves to the next slide.
// A slide that does not include this file works exactly the same, minus the
// step-by-step reveal.
(function () {
  var steps = Array.prototype.slice.call(document.querySelectorAll('.fragment'));
  var shown = 0;

  function next() {
    if (shown >= steps.length) return false;
    steps[shown++].classList.add('on');
    return true;
  }
  function prev() {
    if (shown <= 0) return false;
    steps[--shown].classList.remove('on');
    return true;
  }

  window.addEventListener('message', function (e) {
    var m = e.data;
    if (!m || typeof m.type !== 'string' || !e.source) return;
    var handled;
    if (m.type === 'deckhand:next') handled = next();
    else if (m.type === 'deckhand:prev') handled = prev();
    else return;
    e.source.postMessage({ type: 'deckhand:handled', handled: handled }, '*');
  });

  // Keyboard works too when the slide is opened on its own in a browser.
  window.addEventListener('keydown', function (e) {
    if (e.key === 'ArrowRight' || e.key === ' ') next();
    if (e.key === 'ArrowLeft') prev();
  });

  if (window.parent !== window) {
    window.parent.postMessage({ type: 'deckhand:ready' }, '*');
  }
})();
