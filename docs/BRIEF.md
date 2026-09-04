# Deckhand — brief pour Claude Code

Tu vas créer un projet complet appelé **Deckhand**. Lis ce document en entier avant d'écrire une ligne, puis exécute les jalons dans l'ordre, en t'arrêtant à la fin de chacun pour me montrer ce qui tourne. Ne saute pas de jalon, ne commence pas le Hub avant que le CLI soit utilisable.

## 1. Le produit en trois phrases

Deckhand transforme un dossier de fichiers HTML (un fichier = une slide) en présentation. Il fournit trois écrans synchronisés : la **scène** (projecteur), la **télécommande** (téléphone du présentateur : prev/next, notes, chrono, pointeur laser) et le **public** (n'importe quel appareil qui suit en direct). Un binaire Go unique fait tout en local sur le réseau du présentateur ; le même binaire, en mode `serve`, devient un hub multi-utilisateurs hébergé, payant, qui ajoute les spectateurs distants, le lien permanent et les statistiques.

Public cible : gens de la tech et créateurs qui génèrent leurs slides en HTML (à la main ou via une IA) et ne veulent plus de PowerPoint. Le CLI est open source (MIT). Le Hub est le produit commercial de CODE79 SCOMM (Belgique).

## 2. Décisions techniques, non négociables

- **Langage** : Go 1.23+, un seul module `github.com/stranix/deckhand`, un seul binaire `deckhand`. Pas de CGO. Cross-compile darwin/arm64, darwin/amd64, linux/arm64, linux/amd64, windows/amd64.
- **HTTP** : `net/http` de la stdlib + `github.com/go-chi/chi/v5` pour le routing. WebSocket via `github.com/coder/websocket` (ex nhooyr).
- **Front des trois écrans** : HTML/CSS/JS vanilla, **sans framework, sans build step**, embarqué dans le binaire via `embed`. Pas de React, pas de bundler. Le front doit rester lisible et modifiable par un humain avec un éditeur de texte.
- **Base de données (Hub uniquement)** : PostgreSQL 16 via `pgx/v5`, migrations avec `golang-migrate`, embarquées. Le mode local n'a **aucune** base de données : état en mémoire.
- **Auth Hub** : magic link par e-mail (pas de mot de passe), sessions en cookie HttpOnly/Secure/SameSite=Lax. Pas de OAuth au MVP.
- **Paiement Hub** : Stripe Checkout + webhooks. Abonnement mensuel. Rien d'autre.
- **Sécurité des slides (jour 1, pas de retrofit)** :
  - chaque slide rendue dans une `<iframe sandbox="allow-scripts">` — **jamais** `allow-same-origin` ;
  - CSP stricte sur les pages de l'app ;
  - en mode Hub, les fichiers de decks sont servis depuis une **origine distincte** de l'app (`decks.<domaine>` vs `app.<domaine>`), configurable par variable d'environnement ;
  - taille max d'un deck : 200 Mo, 500 slides ; zip slip et path traversal refusés ; seuls les fichiers `.html`, `.css`, `.js`, `.json`, images, vidéos, polices, `.svg` (servi en `Content-Type: image/svg+xml` avec `Content-Disposition: attachment` sauf pour les `<img>`), `.pdf` sont acceptés.
- **Tests** : `go test ./...` doit passer à chaque jalon. Tests unitaires sur le parsing du deck et le protocole ; test d'intégration qui lance le serveur, ouvre deux WebSockets et vérifie la synchro.
- **Qualité** : `gofmt`, `go vet`, `golangci-lint` (config fournie), pas de warning. Commits conventionnels (`feat:`, `fix:`, `docs:`…). Messages en anglais.
- **Licence** : MIT pour tout ce qui est dans `cmd/`, `internal/deck`, `internal/session`, `internal/local`, `web/`. Le code du Hub (`internal/hub`) est dans le même repo mais sous licence **BSL 1.1** (fichier `LICENSE.hub`) — ça évite deux repos tout en protégeant la partie commerciale.

## 3. Le format de deck (spec à respecter et à documenter dans `docs/FORMAT.md`)

Un deck est un dossier, ou un `.zip` / `.tar.gz` de ce dossier :

```
talk.zip
├── deck.json          # facultatif
├── 01-title.html
├── 02-problem.html
├── ...
└── assets/            # facultatif, libre
```

Règles :
- Sans `deck.json`, toutes les `*.html` à la racine, triées par nom (tri naturel : `2-` avant `10-`), ratio 16:9, sans notes.
- `deck.json` :
  ```json
  {
    "title": "Ship it before lunch",
    "ratio": "16:9",                 // ou "4:3", "16:10"
    "width": 1920,                   // largeur de conception ; hauteur déduite du ratio
    "slides": [
      { "file": "01-title.html", "notes": "Attendre que la salle se calme." },
      { "file": "03-demo.html",  "notes": "Deux minutes, pas plus." }
    ]
  }
  ```
  Si `slides` est présent, il fait autorité (ordre et sélection). Un fichier listé mais absent = erreur au chargement, avec le nom du fichier.
- Chaque slide est un document HTML complet et autonome. Elle est affichée dans une iframe dont le contenu est mis à l'échelle (`transform: scale()`) pour tenir dans l'écran en conservant le ratio, lettrage noir si nécessaire.
- **Protocole optionnel slide ↔ Deckhand**, via `postMessage` (documenté) :
  - Deckhand → slide : `{type:"deckhand:next"}`, `{type:"deckhand:prev"}` pour les slides avec fragments internes ; la slide répond `{type:"deckhand:handled", handled:true|false}`. Si `false` ou pas de réponse en 150 ms, Deckhand change de slide.
  - Slide → Deckhand : `{type:"deckhand:notes", text:"..."}` pour fournir ses notes ; `{type:"deckhand:ready"}`.
  - Une slide qui n'implémente rien fonctionne quand même.
- `deckhand validate <deck>` vérifie tout ça et sort en code 1 avec un rapport lisible.

## 4. Le protocole de session

Une **session** = un deck chargé + un état `{slide: int, fragment: int, pointer: {x,y}|null, startedAt}`. Identifiée par un code court (6 caractères, sans I/O/0/1).

Un seul endpoint WebSocket `/ws/{code}` ; le rôle est donné à la connexion (`?role=stage|remote|viewer`), le rôle `remote` exige un token secret généré au démarrage de la session (dans le QR code du présentateur). Messages JSON, un objet par frame :

```
client → serveur : {op:"goto", slide:3} | {op:"next"} | {op:"prev"} | {op:"pointer", x:0.42, y:0.73} | {op:"pointer", x:null}
serveur → clients : {op:"state", state:{...}}   (envoyé à la connexion et à chaque changement)
                    {op:"deck",  deck:{title, ratio, width, slides:[{index, url, notes?}]}}   (à la connexion ; notes uniquement pour remote)
                    {op:"viewers", count:17}   (toutes les 2 s si changement)
```

Le viewer peut se **détacher** localement (naviguer seul) : c'est côté client, le serveur ne le sait pas. Un bouton « revenir au direct » réaligne sur `state`. Reconnexion automatique avec backoff, rejeu de `state` à la reconnexion. Le stage tolère une coupure de 30 s sans rien changer à l'affichage.

## 5. Les trois écrans (front embarqué, `web/`)

Design : reprends **exactement** les tokens du fichier `site/index.html` que je te fournis (couleurs `--ink #12233B`, `--sea #1E6E74`, `--brass #C9A227`, `--fog #E9EEEF`, police d'interface Bricolage Grotesque avec fallback système, corps Source Serif 4). Les polices ne doivent **pas** être chargées depuis Google Fonts dans les écrans de présentation (ça doit marcher hors ligne) : utilise les fallbacks système, ou embarque les fichiers woff2 si la licence le permet.

- **`/s/{code}` — stage** : plein écran au clic ou touche `f`, aucune interface visible, slide courante centrée, pointeur laser dessiné par-dessus l'iframe (un `<div>` rouge avec halo, positionné en coordonnées normalisées). Touches `→ ← Espace PageUp PageDown Home End` pilotent aussi la session (utile quand le présentateur est au clavier). Touche `b` : écran noir.
- **`/r/{code}?t={token}` — remote** : optimisé iPhone en portrait, mais utilisable partout. Contient : miniatures slide courante et suivante (iframes réduites, `pointer-events:none`), notes de la slide courante, chrono (démarre au premier `next`, réinitialisable), compteur `3 / 24`, boutons prev/next larges (pouce), bouton « laser » qui active un pavé tactile : glisser le doigt sur la miniature de la slide courante déplace le pointeur sur la scène. QR code du lien public à afficher à la salle (bouton « montrer le QR » qui l'affiche en plein écran sur le **stage** pendant 15 s). Vibration haptique sur changement de slide si disponible. Un `<meta name="apple-mobile-web-app-capable">` et un `manifest.json` pour l'ajout à l'écran d'accueil.
- **`/v/{code}` — viewer** : slide courante, indicateur « en direct » ou « détaché », navigation par swipe/flèches, bouton « revenir au direct ». En portrait sur téléphone : la slide en haut au ratio, les notes publiques (si le deck en a de marquées `public: true`) en dessous. Compte des spectateurs affiché.

Tout est bilingue FR/EN : détection `navigator.language`, chaînes dans un objet JS, pas de lib i18n.

## 6. Le CLI (`cmd/deckhand`)

```
deckhand present <deck.zip|dossier> [--port 7777] [--open] [--hub https://…] [--no-lan]
deckhand validate <deck>
deckhand push <deck> --hub https://… [--slug ship-it]         # Hub uniquement
deckhand serve [--addr :8080] [--pg …] [--deck-origin …]      # démarre le Hub
deckhand version
```

`present` :
1. charge et valide le deck ;
2. démarre le serveur sur toutes les interfaces, port 7777 par défaut, en trouvant un port libre sinon ;
3. affiche dans le terminal : l'URL de la scène, l'URL de la télécommande (avec token), l'URL du public, et **deux QR codes ASCII** (télécommande et public) ; l'IP LAN choisie doit être la bonne (ignore les interfaces docker/vpn/link-local ; permet `--ip` pour forcer) ;
4. `--open` ouvre la scène dans le navigateur par défaut ;
5. si `--hub` est donné et qu'un token existe dans `~/.config/deckhand/config.toml`, pousse le deck vers le Hub puis maintient une connexion WebSocket sortante qui relaie chaque `state` ; le Hub sert les viewers distants ; la scène et la télécommande restent locales. Coupure du Hub = log en jaune, présentation locale inchangée.

Sortie de terminal soignée (couleurs, alignement), messages d'erreur qui disent quoi faire.

## 7. Le Hub (`internal/hub`) — jalon 4 et suivants

Multi-tenant. Tables : `users`, `sessions_auth`, `decks` (slug unique par user, versions), `presentations` (une instance live d'un deck, code, state, started_at, ended_at), `viewers_events` (présentation, viewer_id anonyme, slide, timestamp — pour les stats), `subscriptions` (Stripe).

Routes :
- `/` → landing (le `site/index.html` fourni, servi tel quel) ;
- `/login`, `/auth/callback` (magic link) ;
- `/app` → liste des decks, upload (zip), bouton « présenter maintenant » (démarre une session hébergée pilotable depuis `/r/...` comme en local — sans CLI) ;
- `/d/{user}/{slug}` → lien permanent d'un deck (dernière version), mode viewer détaché, indexable ;
- `/v/{code}`, `/r/{code}`, `/s/{code}` → identiques au local, mais `code` est résolu en base ;
- `/api/v1/decks` (push CLI, auth par token), `/api/v1/relay/{code}` (WebSocket entrant du CLI) ;
- `/billing` → Stripe Checkout / portail client ; `/webhooks/stripe`.

Stats après une présentation : nombre de spectateurs uniques, courbe des spectateurs présents par slide, temps passé par slide. Une page simple, un graphique en SVG généré côté serveur (pas de lib JS de charts).

Limites du plan gratuit Hub (utilisateur connecté sans abonnement) : 1 deck, 10 spectateurs distants, lien permanent 7 jours. Abonnement : illimité. Les valeurs sont dans la config, pas dans le code.

Déploiement : un `Dockerfile` multi-stage (image finale `distroless`), un `docker-compose.hub.yml` avec PostgreSQL 16 et le hub, variables d'environnement documentées dans `docs/HUB.md`, healthcheck `/healthz`, métriques Prometheus sur `/metrics` (connexions WS ouvertes, sessions actives, viewers, latence de relay).

## 8. Le site (`site/`)

Je te fournis `site/index.html` (landing bilingue, déjà écrite). Ne réécris pas son contenu ; intègre-le tel quel comme page d'accueil du Hub et publie-le aussi en statique. Ajoute uniquement : `docs/` rendus en HTML (FORMAT, CLI, HUB, PROTOCOL) avec la même charte, une page `/changelog` générée depuis `CHANGELOG.md`, et les balises Open Graph.

## 9. Structure du repo

```
deckhand/
├── cmd/deckhand/           main.go, sous-commandes (cobra)
├── internal/
│   ├── deck/               parsing, validation, zip/tar, deck.json, tri naturel
│   ├── session/            état, protocole WS, hub de diffusion
│   ├── local/              serveur `present`, découverte IP, QR ASCII
│   ├── hub/                serveur `serve`, auth, decks, stats, stripe, relay
│   └── ui/                 handlers qui servent web/ embarqué
├── web/                    stage/, remote/, viewer/, shared/ (HTML/CSS/JS vanilla)
├── site/                   landing + docs statiques
├── docs/                   FORMAT.md, CLI.md, HUB.md, PROTOCOL.md, SECURITY.md
├── examples/               ship-it/ (le deck de démo, 8 slides, avec deck.json et une slide à fragments)
├── migrations/
├── .github/workflows/      ci.yml (test+lint), release.yml (goreleaser, binaires + Homebrew tap)
├── Dockerfile, docker-compose.hub.yml, .goreleaser.yaml, .golangci.yml
├── LICENSE (MIT), LICENSE.hub (BSL 1.1), README.md, CHANGELOG.md, CLAUDE.md
└── Makefile                build, test, lint, run-local, run-hub, release
```

Crée le dépôt git, la branche `main`, un premier commit par jalon. Remote : `git@github.com:stranix/deckhand.git` (je le crée sur GitHub ; si le push échoue, dis-le-moi et continue en local). `CLAUDE.md` à la racine doit contenir les décisions de la section 2 et les commandes Makefile pour que tu les retrouves à chaque session.

## 10. Jalons — arrête-toi à la fin de chacun

**Jalon 1 — Deck & validation.** `internal/deck` complet avec tests, `deckhand validate`, `examples/ship-it`. Critère : `deckhand validate examples/ship-it` sort 0 ; un zip avec `../` sort 1 avec un message clair.

**Jalon 2 — Present local, stage + remote.** Serveur, WebSocket, stage et remote fonctionnels, QR codes dans le terminal. Critère : je lance `deckhand present examples/ship-it --open`, je scanne le QR avec mon iPhone, je change de slide depuis le téléphone, le laser bouge sur la scène. Test d'intégration WS vert.

**Jalon 3 — Viewer + finitions locales.** Écran public, mode détaché, reconnexion, QR du public affichable sur la scène, chrono, écran noir, écran d'accueil iOS. `go test`, `golangci-lint`, `goreleaser --snapshot` verts. README avec GIF ou capture. Critère : `brew install stranix/tap/deckhand` fonctionne en local via `brew install --build-from-source` sur le snapshot. **Ici, on publie v0.1.0.**

**Jalon 4 — Hub : auth, decks, viewers distants, relay.** Sans paiement ni stats. Critère : `deckhand present … --hub https://deckhand.stranix.net` relaie, un viewer sur réseau mobile suit ma présentation locale, le lien `/d/gilles/ship-it` reste consultable après.

**Jalon 5 — Hub : stats, Stripe, limites, métriques, déploiement.** Critère : docker compose up sur un VPS neuf, healthz vert, un abonnement test Stripe passe, les limites du plan gratuit s'appliquent.

**Jalon 6 — Site et docs.** Landing servie, docs rendues, changelog, OG. **v1.0.0.**

## 11. Ce que tu ne fais PAS

- Pas d'app iOS/Android native. PWA uniquement.
- Pas de convertisseur PPTX au MVP (juste un stub `deckhand import` qui explique que c'est prévu).
- Pas d'éditeur de slides. Deckhand présente, il ne crée pas.
- Pas d'analytics tiers, pas de cookies non essentiels, pas de tracking sur le site.
- Pas de dépendance JS côté front. Si tu penses en avoir besoin, propose-la et attends ma réponse.

## 12. Comment travailler avec moi

- Français dans la conversation, anglais dans le code, les commits et la doc (la landing est bilingue, les docs techniques sont en anglais uniquement).
- Quand une décision n'est pas couverte ici, prends la plus simple, note-la dans `docs/DECISIONS.md` avec la date et une ligne de justification, et continue. Ne me pose une question que si elle bloque le jalon.
- À la fin de chaque jalon : résumé en 10 lignes max de ce qui existe, comment le tester, et ce qui reste ouvert.
- Je suis SysAdmin/DBA, pas développeur front : commente le JS des trois écrans comme si tu l'expliquais à quelqu'un qui devra le maintenir sans toi.
