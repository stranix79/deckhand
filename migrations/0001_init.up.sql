-- Deckhand hub, initial schema (brief §7).

CREATE TABLE users (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email       text NOT NULL UNIQUE,
    handle      text NOT NULL UNIQUE,          -- /d/{handle}/{slug}
    created_at  timestamptz NOT NULL DEFAULT now(),
    last_login  timestamptz
);

-- Magic links, browser cookies and CLI API tokens. Only a hash is stored.
CREATE TABLE sessions_auth (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        text NOT NULL CHECK (kind IN ('magic', 'cookie', 'api')),
    token_hash  bytea NOT NULL UNIQUE,
    label       text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz,
    used_at     timestamptz
);
CREATE INDEX sessions_auth_user ON sessions_auth(user_id);

-- A deck: one slug per user, a version counter, the parsed manifest and the
-- storage path of the current version.
CREATE TABLE decks (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    slug        text NOT NULL,
    title       text NOT NULL,
    version     int  NOT NULL DEFAULT 1,
    ratio       text NOT NULL,
    width       int  NOT NULL,
    slide_count int  NOT NULL,
    size_bytes  bigint NOT NULL,
    path        text NOT NULL,                 -- directory of the current version
    permalink_code text UNIQUE,                -- session code of /d/{user}/{slug}
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz,                   -- free plan: permanent link lifetime
    UNIQUE (user_id, slug)
);

-- One live instance of a deck.
CREATE TABLE presentations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    deck_id     uuid NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code        text NOT NULL UNIQUE,
    token       text NOT NULL,
    mode        text NOT NULL CHECK (mode IN ('hosted', 'relay', 'permalink')),
    state       jsonb NOT NULL DEFAULT '{}'::jsonb,
    started_at  timestamptz NOT NULL DEFAULT now(),
    ended_at    timestamptz
);
CREATE INDEX presentations_user ON presentations(user_id, started_at DESC);

-- Anonymous viewer events for the statistics page.
CREATE TABLE viewers_events (
    id              bigserial PRIMARY KEY,
    presentation_id uuid NOT NULL REFERENCES presentations(id) ON DELETE CASCADE,
    viewer_id       text NOT NULL,
    kind            text NOT NULL CHECK (kind IN ('join', 'leave', 'slide')),
    slide           int  NOT NULL,
    at              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX viewers_events_presentation ON viewers_events(presentation_id, at);

-- Stripe subscription, one per user.
CREATE TABLE subscriptions (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    stripe_customer_id     text NOT NULL,
    stripe_subscription_id text,
    status                 text NOT NULL DEFAULT 'none',
    current_period_end     timestamptz,
    updated_at             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX subscriptions_customer ON subscriptions(stripe_customer_id);
