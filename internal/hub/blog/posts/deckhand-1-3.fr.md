---
title: Deckhand 1.3 : une démo de quinze secondes, des pages comparatives et un prompt pour votre LLM
date: 2026-09-23
summary: Trois jours après la 1.0, une version qui explique plutôt qu'elle n'ajoute. Ce qui change sur le site et dans le binaire, et comment mettre à jour.
image: stage.jpg
credit: Photo Jorge Jaramillo, Wikimania 2026 Paris, CC BY-SA 4.0.
---

Les premiers retours après le lancement ne parlaient pas de fonctionnalités. C'était « je ne comprends pas ce que ça fait » et « c'est quoi la différence avec reveal.js ? ». Justifié. La 1.3 est donc une version qui explique, avec un correctif en 1.3.1 que la chaîne de release m'a imposé.

## La démo, en quinze secondes

La page d'accueil s'ouvre maintenant sur un enregistrement d'écran des trois écrans : la scène à gauche, la télécommande sur un téléphone, la vue du public à droite, avec le laser et le QR code en action. Pas une maquette, la vraie chose, servie par le hub lui-même (`/static/site/demo.mp4`, avec le support des requêtes Range pour que Safari la lise). Le même enregistrement est en [GIF dans le README](https://github.com/stranix79/deckhand#readme).

## Deckhand face aux autres

Trois pages honnêtes, avec un tableau et une section « quand l'autre est le meilleur choix » : [reveal.js](/vs/reveal-js), [Slidev](/vs/slidev) et [Google Slides](/vs/google-slides), plus une [vue d'ensemble](/vs). Version courte : eux fabriquent des slides, Deckhand présente le HTML que vous avez déjà. S'il vous faut des thèmes, des plugins et un export PDF, reveal.js est le bon outil. S'il vous faut présenter un dossier depuis n'importe quel écran avec le téléphone en main, c'est Deckhand.

## Un prompt pour votre modèle de langage

La plupart de mes decks sont générés. Voici donc [le prompt](/docs/LLM) qui fait produire à n'importe quel modèle un dossier qui passe `deckhand validate` du premier coup : un fichier par slide, le nommage, le `deck.json`, les règles sur les scripts et les ressources. Collez-le, décrivez votre talk, présentez.

## Sous le capot

- `/sitemap.xml` et `/robots.txt` ; les pages de doc, le changelog et les comparatifs portent une description, une URL canonique et des balises Open Graph, donc ils sont indexables et se partagent avec un vrai aperçu. Les pages de l'application et les écrans en direct restent hors des moteurs.
- 1.3.1 : le changelog est maintenant embarqué depuis la racine du dépôt. Les builds CI, goreleaser et Homebrew ne le copiaient jamais, leurs binaires répondaient 404 sur `/changelog` et la chaîne de release échouait précisément sur ce test. Corrigé, avec un test qui cherche la balise robots exacte au lieu du mot « noindex » (que le texte du changelog contient lui-même).

## Mettre à jour

```
brew upgrade deckhand            # Homebrew
deckhand version                 # doit répondre 1.3.1
```

Ou prenez le binaire de votre plateforme dans les [releases](https://github.com/stranix79/deckhand/releases). Hubs auto-hébergés : changez le tag d'image et `docker compose up -d`, les migrations s'appliquent au démarrage.

Le détail est dans le [changelog](/changelog).
