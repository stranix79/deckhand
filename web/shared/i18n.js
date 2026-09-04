// Tiny bilingual helper. No library: one object per language, a t() function,
// and data-i18n attributes in the HTML that are filled at load.
//
// Usage in JS:   t('next')            → "Suivant" or "Next"
// Usage in HTML: <button data-i18n="next"></button>
//                <input data-i18n-placeholder="code">
'use strict';

const I18N = {
  en: {
    stage: 'Stage', remote: 'Remote', viewer: 'Audience',
    next: 'Next', prev: 'Previous', laser: 'Laser', black: 'Black screen',
    showQr: 'Show QR', hideQr: 'Hide QR', backToLive: 'Back to live',
    live: 'Live', detached: 'Detached', connecting: 'Connecting…',
    reconnecting: 'Reconnecting…', offline: 'Connection lost',
    viewers: 'viewers', viewer1: 'viewer', notes: 'Notes', noNotes: 'No notes for this slide.',
    resetTimer: 'Tap to reset the timer', timerIdle: 'Timer starts on the first “next”',
    scanToFollow: 'Scan to follow on your phone', endOfDeck: 'End of deck',
    fullscreenHint: 'Click or press F for fullscreen · B for black screen',
    waitingForDeck: 'Waiting for the deck…', notFound: 'This session does not exist or has ended.',
    forbidden: 'This remote link is invalid (missing or wrong token).',
    of: 'of', slide: 'Slide', publicNotes: 'Notes', tapToStart: 'Tap to start',
    addToHome: 'Tip: add this page to your home screen for a full-screen remote.',
    swipeHint: 'Swipe or use ← → to browse on your own',
  },
  fr: {
    stage: 'Scène', remote: 'Télécommande', viewer: 'Public',
    next: 'Suivant', prev: 'Précédent', laser: 'Laser', black: 'Écran noir',
    showQr: 'Montrer le QR', hideQr: 'Cacher le QR', backToLive: 'Revenir au direct',
    live: 'En direct', detached: 'Détaché', connecting: 'Connexion…',
    reconnecting: 'Reconnexion…', offline: 'Connexion perdue',
    viewers: 'spectateurs', viewer1: 'spectateur', notes: 'Notes', noNotes: 'Pas de notes pour cette slide.',
    resetTimer: 'Toucher pour remettre le chrono à zéro', timerIdle: 'Le chrono démarre au premier « suivant »',
    scanToFollow: 'Scannez pour suivre sur votre téléphone', endOfDeck: 'Fin du deck',
    fullscreenHint: 'Clic ou touche F pour le plein écran · B pour l’écran noir',
    waitingForDeck: 'En attente du deck…', notFound: 'Cette session n’existe pas ou est terminée.',
    forbidden: 'Ce lien de télécommande est invalide (token manquant ou incorrect).',
    of: 'sur', slide: 'Slide', publicNotes: 'Notes', tapToStart: 'Toucher pour démarrer',
    addToHome: 'Astuce : ajoutez cette page à l’écran d’accueil pour une télécommande plein écran.',
    swipeHint: 'Glissez ou utilisez ← → pour naviguer seul',
  },
};

// French if the browser says so, English otherwise. ?lang=fr|en forces it.
const LANG = (function () {
  const forced = new URLSearchParams(location.search).get('lang');
  if (forced === 'fr' || forced === 'en') return forced;
  return (navigator.language || 'en').toLowerCase().startsWith('fr') ? 'fr' : 'en';
})();

function t(key) {
  return (I18N[LANG] && I18N[LANG][key]) || I18N.en[key] || key;
}

// Fill every element carrying data-i18n / data-i18n-title / data-i18n-placeholder.
function applyI18n(root) {
  document.documentElement.lang = LANG;
  (root || document).querySelectorAll('[data-i18n]').forEach(function (el) {
    el.textContent = t(el.getAttribute('data-i18n'));
  });
  (root || document).querySelectorAll('[data-i18n-title]').forEach(function (el) {
    el.title = t(el.getAttribute('data-i18n-title'));
    el.setAttribute('aria-label', el.title);
  });
}
