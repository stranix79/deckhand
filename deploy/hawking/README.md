# Déployer le hub sur hawking

Même recette que chutag : image construite sur hawking, conteneurs derrière
le nginx partagé de Ghost, cert Let's Encrypt. Deux noms DNS, un seul
certificat (SAN).

## Première fois

```
# 1. DNS (depuis le Mac) : CNAME deckhand + decks.deckhand → hawking.stranix.net
cd ~/git/stranix/stranix-git/terraform/cloudflare-dns && terraform apply \
  -target=cloudflare_record.stranix_deckhand_cname -target=cloudflare_record.stranix_deckhand_decks_cname

# 2. Fichiers sur hawking : compose + .env dans stranix-git
#    docker-compose/hawking/stranix/deckhand/ ; vhost dans shared/nginx/conf.d/.
#    .env : POSTGRES_PASSWORD et DECKHAND_SECRET (openssl rand -hex 32),
#    DECKHAND_STRIPE_* quand Stripe est prêt.
ssh stranix@hawking.code79.com 'sudo mkdir -p /var/lib/deckhand/decks /var/lib/deckhand/postgres && sudo chown 65532:65532 /var/lib/deckhand/decks'

# 3. Image
rsync -az --delete --exclude .git --exclude .pg --exclude dist --exclude /deckhand ~/git/stranix/deckhand/ stranix@hawking.code79.com:/tmp/deckhand/
ssh stranix@hawking.code79.com 'docker build --build-arg VERSION=0.2.0 -t deckhand:0.2.0 /tmp/deckhand'

# 4. Cert (AVANT de charger le vhost)
ssh stranix@hawking.code79.com 'docker exec ghost_certbot certbot certonly --webroot -w /var/www/certbot \
  -d deckhand.stranix.net -d decks.deckhand.stranix.net --cert-name deckhand.stranix.net -n --agree-tos -m stranix79@gmail.com'

# 5. Up + reload nginx
ssh stranix@hawking.code79.com 'cd /opt/stranix-git/docker-compose/hawking/stranix/deckhand && docker compose up -d \
  && docker exec ghost_nginx nginx -t && docker exec ghost_nginx nginx -s reload'
curl -s https://deckhand.stranix.net/healthz
```

Le conteneur tourne en `nonroot` (uid 65532) : le dossier des decks doit lui
appartenir (étape 2).

## Mise à jour

Bumper la version dans `image:` (compose) et `--build-arg VERSION`, rsync +
build, `docker compose up -d`. Les migrations SQL s'appliquent au démarrage.

## Données

Postgres dans `/var/lib/deckhand/postgres`, decks dans `/var/lib/deckhand/decks`.
Dump : `docker exec deckhand_postgres pg_dump -U deckhand deckhand | gzip > deckhand-$(date +%F).sql.gz`.

## Stripe

Créer le produit « Deckhand Pro » (prix mensuel), le webhook
`https://deckhand.stranix.net/webhooks/stripe` (événements
`checkout.session.completed`, `customer.subscription.*`), puis mettre
`DECKHAND_STRIPE_SECRET_KEY`, `DECKHAND_STRIPE_WEBHOOK_SECRET`,
`DECKHAND_STRIPE_PRICE_ID` dans `.env` et `docker compose up -d`.
