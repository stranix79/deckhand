---
title: Deckhand 1.0 est là. Vos slides sont déjà des pages web, présentez-les comme telles.
date: 2026-09-22
summary: Un binaire Go, un dossier de slides HTML, trois écrans. Pourquoi je l'ai écrit, ce qu'il fait, et ce que j'aimerais entendre de vous.
image: launch.jpg
credit: Photo SpaceX, lancement JCSAT-16, CC0.
---

Pendant des années, mes présentations ont tenu à un câble HDMI, à un Keynote qui décide de ne plus s'ouvrir, et à la question rituelle : « tu as tes slides en PDF au cas où ? ». Je fais de l'infra. Je passe mes journées à rendre des systèmes fiables. Et je présentais avec le maillon le plus fragile de la pièce.

## Mes slides étaient déjà du HTML

Depuis un an, je ne fais plus de slides à la main. Je décris le contenu, un outil (ou un assistant) me sort une page HTML par slide, propre, avec du code coloré et des schémas SVG. Le problème n'est plus de produire les slides, c'est de les présenter : les afficher sur le grand écran, avancer sans retourner au laptop, avoir mes notes sous les yeux, et laisser les gens du fond (ou de chez eux) suivre sur leur propre écran.

Aucun outil ne faisait les trois à la fois sans compte, sans framework et sans dépendre d'un service. Alors j'en ai écrit un.

## Un dossier, un binaire, trois écrans

Deckhand prend un dossier de fichiers HTML (un par slide, plus un petit `deck.json`) et vous donne :

- **la scène** : la slide en plein écran, sur n'importe quel navigateur, donc sur n'importe quel écran ;
- **la télécommande** : votre téléphone, avec les miniatures, les notes du présentateur, un chrono et un pointeur laser ;
- **le public** : un lien que les gens ouvrent sur leur appareil et qui suit la scène en direct.

```
deckhand validate talk/     # vérifie le deck et liste tout ce qui cloche
deckhand present  talk/     # scène + télécommande + public sur le réseau local
deckhand push     talk/     # publie sur un hub et donne un lien permanent
```

Un seul binaire Go, macOS, Linux ou Windows. Pas de compte pour présenter en local : la commande affiche deux QR codes, vous scannez la télécommande avec le téléphone, vous ouvrez la scène sur l'écran, c'est parti. Le tout est synchronisé par WebSocket, fragments et laser compris.

## La sécurité d'abord, parce que c'est mon métier

Servir du HTML qu'on n'a pas écrit soi-même, c'est du XSS en libre-service si on ne fait pas attention. Chaque slide tourne dans une `<iframe sandbox="allow-scripts">`, jamais `allow-same-origin` : le script d'une slide ne voit ni les cookies, ni la session, ni les autres slides. Les pages de l'application ont une CSP stricte. Sur le hub, les decks sont servis depuis une origine séparée. Les archives sont vérifiées à l'import : pas de chemin qui remonte, types de fichiers limités, taille plafonnée. Des règles ennuyeuses, présentes dès la première version.

## Le hub, pour ceux qui veulent partager

Présenter en local suffit dans une salle. Pour les gens à distance ou pour laisser un lien après la conférence, il y a [deckhand.show](/) : vous poussez le deck, vous obtenez un lien permanent, les spectateurs suivent en direct depuis n'importe où, et vous voyez combien de personnes ont suivi. Connexion par lien magique, pas de mot de passe. Le hub est aussi [auto-hébergeable](/docs/HUB) : un binaire, un PostgreSQL, un `docker compose` dans le dépôt.

## Ce que je veux entendre

C'est une 1.0. Je n'ai testé que mes propres decks. Si vous faites des présentations, pour le boulot, pour un cours ou pour un meetup, essayez-le et dites-moi ce qui coince. C'est exactement ce que je cherche.

- Installation : `brew install stranix79/tap/deckhand`, ou les binaires dans les [releases](https://github.com/stranix79/deckhand/releases).
- Le code, MIT pour la partie locale : [github.com/stranix79/deckhand](https://github.com/stranix79/deckhand).
- La [newsletter](/#newsletter) si vous voulez être prévenu des prochaines versions, une adresse et c'est tout.
