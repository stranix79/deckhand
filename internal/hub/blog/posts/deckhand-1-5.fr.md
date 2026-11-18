---
title: Deckhand 1.5 : importez votre PowerPoint, et installez en une ligne
date: 2026-11-18
summary: Deckhand présente des slides HTML, et la plupart des slides sont encore dans un .pptx. Il y a maintenant une commande pour ça. Ce qu'elle garde, ce qu'elle perd, et une installation plus courte.
image: import.jpg
---

Deckhand présente un dossier de slides HTML. Très bien si vos slides sont déjà en HTML. La plupart ne le sont pas. Elles dorment dans un .pptx, parfois depuis dix ans, et "il suffit de les refaire" n'est pas une réponse que j'accepterais moi-même.

La 1.5 apporte donc une commande.

## deckhand import

```
deckhand import talk.pptx     # écrit ./talk/
deckhand present talk
```

Vous obtenez un fichier HTML par slide, un `deck.json` avec vos notes de présentateur, et un dossier `assets/` avec les images, les vidéos et un `style.css`. Cette feuille de style est à vous, changez-la. Ce qui sort est un deck normal : vous pouvez l'éditer à la main, lancer `deckhand validate` dessus, le pousser sur un hub.

## Deux sortes de slides

Ma première version lisait les titres et les puces et écrivait du HTML propre. Sur des slides de texte, c'était bien. Puis je lui ai donné un vrai fichier, un deck rempli de schémas d'architecture, et j'ai eu des pages et des pages de listes à puces, avec les libellés des boîtes les uns sous les autres. Inutilisable.

L'import décide donc slide par slide.

- Une **slide de texte** (un titre, des puces, une image, un tableau) devient le HTML que vous auriez écrit à la main : un titre, de vraies listes imbriquées, un vrai `<table>`. Elle prend l'allure de `style.css`, pas celle du thème PowerPoint.
- Un **dessin** (des boîtes, des flèches et des libellés posés à la main) est redessiné là où PowerPoint avait mis les choses : les formes avec leur fond et leur contour, le texte dedans, les connecteurs en lignes SVG avec leurs pointes de flèche. Il garde ses couleurs, y compris avec `--dark`.

Sont gardés aussi : le gras, l'italique et les liens, les niveaux des listes, les tableaux avec cellules fusionnées, les vidéos et les sons embarqués, et le format de la slide.

## Ce qui est perdu

Je préfère l'écrire ici plutôt que vous le découvriez devant une salle.

Pas converti : le thème et les décorations du masque (logos, bandeaux), les animations et les transitions, les graphiques, les SmartArt, les formes libres, les images EMF et WMF, les médias liés et non embarqués. Les slides masquées sont sautées. Le vieux format binaire `.ppt` n'est pas lu, enregistrez-le d'abord en `.pptx`.

Rien ne disparaît sans le dire. Chaque perte est affichée avec son numéro de slide, et elle reste dans la slide sous la forme d'un cadre en pointillés tant que vous ne l'avez pas remplacée.

Un mot sur les polices. Le texte tient le mieux dans ses boîtes quand les polices du fichier sont installées sur la machine qui présente. Sinon c'est la police du système, un peu plus étroite pour que les libellés rentrent quand même.

Tout ça, c'est la bibliothèque standard de Go. Pas de LibreOffice, pas de Python, rien à installer à côté du binaire. Le détail est dans la [doc du CLI](/docs/CLI).

## Installer en une ligne

```
curl -fsSL https://deckhand.show/install.sh | sh
```

Pour macOS et Linux. Le script prend le binaire de la dernière version pour votre machine, vérifie son SHA-256 avec les sommes de contrôle de la release, et le pose dans `/usr/local/bin` ou `~/.local/bin`. Pas de sudo. [Lisez-le](/install.sh) avant de le lancer, il fait une centaine de lignes.

Windows a un zip, et Homebrew marche toujours. Tout est sur la nouvelle [page d'installation](/download/fr). La page d'accueil vous y envoie maintenant, avant elle vous envoyait sur GitHub.

## Aussi, depuis le dernier billet

- Des robots se servaient du formulaire de connexion pour envoyer des liens magiques à des inconnus. Le formulaire a maintenant un pot de miel, un horodatage signé, et un quota par IP et par adresse. Une demande refusée reçoit la page normale "vérifiez votre boîte", donc un robot n'apprend rien. (1.4.7)
- `/llms.txt` et `/llms-full.txt`, pour les modèles de langage qui lisent le site. (1.4.5)

## Mettre à jour

```
brew upgrade deckhand            # Homebrew
deckhand version                 # doit répondre 1.5.1
```

Hubs auto-hébergés : changez le tag d'image et `docker compose up -d`, les migrations s'appliquent au démarrage.

J'ai testé l'import sur mes fichiers à moi, et ce ne sont pas les vôtres. S'il casse sur un de vos decks, ce fichier est exactement ce qu'il me faut : une [issue sur GitHub](https://github.com/stranix79/deckhand/issues) ou un mail.

Le détail est dans le [changelog](/changelog).
