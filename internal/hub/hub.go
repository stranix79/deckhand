package hub

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stranix79/deckhand/internal/session"
	"github.com/stranix79/deckhand/internal/ui"
)

// Hub is the hosted server.
type Hub struct {
	cfg      Config
	db       *pgxpool.Pool
	sessions *session.Manager
	ui       *ui.Server
	tmpl     *template.Template
	metrics  *metrics

	loadMu sync.Mutex // serialises lazy session loading per process
	relay  relayState
	events chan eventRow
}

// Serve runs the hub until ctx ends.
func Serve(ctx context.Context, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "decks"), 0o750); err != nil {
		return err
	}
	db, err := openDB(ctx, cfg.PG)
	if err != nil {
		return err
	}
	defer db.Close()

	h, err := New(cfg, db)
	if err != nil {
		return err
	}
	go h.eventWriter(ctx) //nolint:gosec // server context, not a request
	go h.janitor(ctx)     //nolint:gosec // idem

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           h.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("hub listening", "addr", cfg.Addr, "base_url", cfg.BaseURL, "deck_origin", cfg.DeckOrigin)
		errc <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h.sessions.Each(func(s *session.Session) { s.Close() })
		_ = srv.Shutdown(sctx)
		return nil
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// New builds a hub on an open database (used by Serve and by tests).
func New(cfg Config, db *pgxpool.Pool) (*Hub, error) {
	tmpl, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	h := &Hub{
		cfg:      cfg,
		db:       db,
		sessions: session.NewManager(),
		tmpl:     tmpl,
		metrics:  newMetrics(),
		events:   make(chan eventRow, 1024),
	}
	h.relay.last = map[string]time.Time{}
	h.metrics.bind(h.sessions)
	h.ui = &ui.Server{Lookup: h.Lookup, DeckOrigin: cfg.DeckOrigin}
	return h, nil
}

// Router assembles the app and, when a deck origin is configured, the
// separate deck host. Requests are dispatched on the Host header.
func (h *Hub) Router() http.Handler {
	app := chi.NewRouter()
	app.Use(middleware.Recoverer, h.metrics.middleware, secureHeaders)
	app.Use(middleware.Compress(5, "text/html", "text/css", "text/javascript", "application/json", "image/svg+xml"))
	h.appRoutes(app)

	deckHost := h.cfg.DeckOriginHost()
	if deckHost == "" {
		h.ui.DeckRoutes(app)
		return app
	}
	decks := chi.NewRouter()
	decks.Use(middleware.Recoverer, h.metrics.middleware)
	h.ui.DeckRoutes(decks)
	decks.Get("/healthz", healthz)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Host, deckHost) {
			decks.ServeHTTP(w, r)
			return
		}
		app.ServeHTTP(w, r)
	})
}

func (h *Hub) appRoutes(r chi.Router) {
	r.Get("/healthz", healthz)
	r.Handle("/metrics", h.metrics.handler())
	h.ui.AppRoutes(r)

	// Site and docs (milestone 6) — always available.
	h.siteRoutes(r)

	// Auth.
	r.Get("/login", h.loginPage)
	r.With(h.sameOrigin).Post("/login", h.loginPost)
	r.Get("/auth/callback", h.authCallback)
	r.With(h.sameOrigin).Post("/logout", h.logout)

	// App (browser, cookie).
	r.Route("/app", func(r chi.Router) {
		r.Use(h.requireUser, h.sameOrigin)
		r.Get("/", h.appPage)
		r.Post("/decks", h.uploadDeckForm)
		r.Post("/decks/{id}/present", h.presentNow)
		r.Post("/decks/{id}/delete", h.deleteDeckForm)
		r.Post("/presentations/{id}/end", h.endPresentationForm)
		r.Get("/presentations/{id}/stats", h.statsPage)
		r.Get("/presentations/{id}/stats.svg", h.statsSVG)
		r.Post("/token", h.newAPIToken)
	})
	r.Route("/billing", func(r chi.Router) {
		r.Use(h.requireUser)
		r.Get("/", h.billingPage)
		r.With(h.sameOrigin).Post("/checkout", h.billingCheckout)
		r.With(h.sameOrigin).Post("/portal", h.billingPortal)
	})
	r.Post("/webhooks/stripe", h.stripeWebhook)

	// Permanent deck links.
	r.Get("/d/{handle}/{slug}", h.deckPermalink)

	// API (CLI, Bearer token).
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(h.requireAPI)
		r.Get("/me", h.apiMe)
		r.Post("/decks", h.apiPushDeck)
		r.Post("/presentations", h.apiStartPresentation)
	})
	r.Get("/api/v1/relay/{code}", h.relayWS) // token in the query, checked there
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte("ok\n"))
}

// secureHeaders applies the non-CSP headers to every app response. Pages
// set their own CSP (the screens in ui, the site and app pages in pages.go).
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

// sameOrigin is the CSRF guard for cookie-authenticated POSTs: the Origin
// (or Referer) must be our own base URL. API calls use Bearer tokens and are
// not subject to it.
func (h *Hub) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			origin := r.Header.Get("Origin")
			if origin == "" {
				origin = r.Header.Get("Referer")
			}
			if origin == "" || !strings.HasPrefix(origin, h.cfg.BaseURL) {
				http.Error(w, "cross-origin request refused", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
