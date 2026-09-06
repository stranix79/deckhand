// Package hub is `deckhand serve`: the hosted, multi-tenant Deckhand.
//
// This package is licensed under the Business Source License 1.1, see
// LICENSE.hub at the repository root. Everything else in the repository is MIT.
package hub

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config comes from the environment (documented in docs/HUB.md). Flags of
// `deckhand serve` override the three most common values.
type Config struct {
	Addr       string // DECKHAND_ADDR, ":8080"
	PG         string // DECKHAND_PG, postgres://…
	BaseURL    string // DECKHAND_BASE_URL, https://deckhand.example.com (no trailing slash)
	DeckOrigin string // DECKHAND_DECK_ORIGIN, https://decks.deckhand.example.com ("" = same origin, dev only)
	DataDir    string // DECKHAND_DATA_DIR, where uploaded decks are extracted
	Secret     string // DECKHAND_SECRET, ≥ 32 chars, signs nothing yet but reserved (cookies are random tokens)

	MailHost, MailUser, MailPassword, MailFrom string // MAIL_* (same names as the other stranix stacks)
	MailPort                                   int

	FreeMaxDecks   int           // DECKHAND_FREE_MAX_DECKS, 1
	FreeMaxViewers int           // DECKHAND_FREE_MAX_VIEWERS, 10
	FreeLinkDays   int           // DECKHAND_FREE_LINK_DAYS, 7
	MaxDeckBytes   int64         // DECKHAND_MAX_DECK_BYTES, 200 MB
	MagicLinkTTL   time.Duration // DECKHAND_MAGIC_LINK_TTL, 15m
	CookieTTL      time.Duration // DECKHAND_COOKIE_TTL, 720h

	// Billing: "stripe" (Checkout + webhooks), "odoo" (Odoo Subscriptions,
	// synced over XML-RPC) or "" (no billing: the plan page says so).
	Billing string // DECKHAND_BILLING

	OdooURL, OdooDB, OdooUser, OdooPassword string        // ODOO_URL, ODOO_DB, ODOO_USER, ODOO_PASSWORD
	OdooProductURL, OdooPortalURL           string        // ODOO_PRODUCT_URL (shop page of the Pro product), ODOO_PORTAL_URL (default <url>/my/subscriptions)
	OdooSyncInterval                        time.Duration // ODOO_SYNC_INTERVAL, 5m

	StripeSecretKey     string // DECKHAND_STRIPE_SECRET_KEY
	StripeWebhookSecret string // DECKHAND_STRIPE_WEBHOOK_SECRET
	StripePriceID       string // DECKHAND_STRIPE_PRICE_ID (monthly price)

	DevLogMagicLinks bool // DECKHAND_DEV_LOG_MAGIC_LINKS=1: print links instead of mailing them
}

// FromEnv reads the configuration. Missing optional values get defaults;
// the result is checked by Validate.
func FromEnv() Config {
	c := Config{
		Addr:           env("DECKHAND_ADDR", ":8080"),
		PG:             env("DECKHAND_PG", ""),
		BaseURL:        strings.TrimRight(env("DECKHAND_BASE_URL", "http://localhost:8080"), "/"),
		DeckOrigin:     strings.TrimRight(env("DECKHAND_DECK_ORIGIN", ""), "/"),
		DataDir:        env("DECKHAND_DATA_DIR", "./data"),
		Secret:         env("DECKHAND_SECRET", ""),
		MailHost:       env("MAIL_HOST", ""),
		MailPort:       envInt("MAIL_PORT", 587),
		MailUser:       env("MAIL_USER", ""),
		MailPassword:   env("MAIL_PASSWORD", ""),
		MailFrom:       strings.Trim(env("MAIL_FROM", "Deckhand <no-reply@localhost>"), `"`),
		FreeMaxDecks:   envInt("DECKHAND_FREE_MAX_DECKS", 1),
		FreeMaxViewers: envInt("DECKHAND_FREE_MAX_VIEWERS", 10),
		FreeLinkDays:   envInt("DECKHAND_FREE_LINK_DAYS", 7),
		MaxDeckBytes:   int64(envInt("DECKHAND_MAX_DECK_MB", 200)) << 20,
		MagicLinkTTL:   envDuration("DECKHAND_MAGIC_LINK_TTL", 15*time.Minute),
		CookieTTL:      envDuration("DECKHAND_COOKIE_TTL", 30*24*time.Hour),

		Billing:          env("DECKHAND_BILLING", ""),
		OdooURL:          strings.TrimRight(env("ODOO_URL", ""), "/"),
		OdooDB:           env("ODOO_DB", ""),
		OdooUser:         env("ODOO_USER", ""),
		OdooPassword:     env("ODOO_PASSWORD", ""),
		OdooProductURL:   env("ODOO_PRODUCT_URL", ""),
		OdooPortalURL:    env("ODOO_PORTAL_URL", ""),
		OdooSyncInterval: envDuration("ODOO_SYNC_INTERVAL", 5*time.Minute),

		StripeSecretKey:     env("DECKHAND_STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret: env("DECKHAND_STRIPE_WEBHOOK_SECRET", ""),
		StripePriceID:       env("DECKHAND_STRIPE_PRICE_ID", ""),
		DevLogMagicLinks:    env("DECKHAND_DEV_LOG_MAGIC_LINKS", "") == "1",
	}
	return c
}

// Validate refuses a configuration that cannot work.
func (c Config) Validate() error {
	if c.PG == "" {
		return fmt.Errorf("DECKHAND_PG (or --pg) is required: postgres://user:pass@host/db")
	}
	if u, err := url.Parse(c.BaseURL); err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("DECKHAND_BASE_URL must be an absolute URL, got %q", c.BaseURL)
	}
	if c.DeckOrigin != "" {
		u, err := url.Parse(c.DeckOrigin)
		if err != nil || u.Scheme == "" || u.Host == "" || u.Path != "" {
			return fmt.Errorf("DECKHAND_DECK_ORIGIN must be an origin like https://decks.example.com, got %q", c.DeckOrigin)
		}
	}
	if len(c.Secret) < 32 {
		return fmt.Errorf("DECKHAND_SECRET must be at least 32 characters (openssl rand -hex 32)")
	}
	switch c.Billing {
	case "", "stripe":
	case "odoo":
		if c.OdooURL == "" || c.OdooDB == "" || c.OdooUser == "" || c.OdooPassword == "" || c.OdooProductURL == "" {
			return fmt.Errorf("DECKHAND_BILLING=odoo needs ODOO_URL, ODOO_DB, ODOO_USER, ODOO_PASSWORD and ODOO_PRODUCT_URL")
		}
	default:
		return fmt.Errorf("DECKHAND_BILLING must be \"stripe\", \"odoo\" or empty, got %q", c.Billing)
	}
	if c.MailHost == "" && !c.DevLogMagicLinks {
		return fmt.Errorf("MAIL_HOST is required to send magic links (or set DECKHAND_DEV_LOG_MAGIC_LINKS=1 for development)")
	}
	return nil
}

// Secure is true when the public URL is https (cookies get the Secure flag).
func (c Config) Secure() bool { return strings.HasPrefix(c.BaseURL, "https://") }

// DeckOriginHost is the Host header that selects the deck file server.
func (c Config) DeckOriginHost() string {
	if c.DeckOrigin == "" {
		return ""
	}
	u, _ := url.Parse(c.DeckOrigin)
	return u.Host
}

// StripeEnabled is true when Stripe billing is configured.
func (c Config) StripeEnabled() bool {
	return c.Billing != "odoo" && c.StripeSecretKey != "" && c.StripeWebhookSecret != "" && c.StripePriceID != ""
}

// OdooEnabled is true when Odoo billing is configured.
func (c Config) OdooEnabled() bool { return c.Billing == "odoo" && c.OdooURL != "" }

// OdooPortal is the customer portal link for "manage subscription".
func (c Config) OdooPortal() string {
	if c.OdooPortalURL != "" {
		return c.OdooPortalURL
	}
	return c.OdooURL + "/my/subscriptions"
}

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDuration(k string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
