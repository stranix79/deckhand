package hub

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// User is the authenticated account.
type User struct {
	ID     string
	Email  string
	Handle string
	Paid   bool // active subscription
}

type ctxKey int

const userKey ctxKey = 1

const cookieName = "dh_session"

func newRawToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func hashToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// createAuth stores a new token of the given kind and returns the raw value
// (shown once: in the e-mail, in the cookie, or on the token page).
func (h *Hub) createAuth(ctx context.Context, userID, kind, label string, ttl time.Duration) (string, error) {
	raw := newRawToken()
	var exp *time.Time
	if ttl > 0 {
		t := time.Now().Add(ttl)
		exp = &t
	}
	_, err := h.db.Exec(ctx, `INSERT INTO sessions_auth (user_id, kind, token_hash, label, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		userID, kind, hashToken(raw), label, exp)
	if err != nil {
		return "", err
	}
	return raw, nil
}

// lookupAuth resolves a raw token of a kind to its user. Magic links are
// single-use: the first lookup marks them used.
func (h *Hub) lookupAuth(ctx context.Context, raw, kind string) (*User, error) {
	if raw == "" {
		return nil, pgx.ErrNoRows
	}
	var u User
	var authID string
	err := h.db.QueryRow(ctx, `
		SELECT a.id, u.id, u.email, u.handle,
		       EXISTS (SELECT 1 FROM subscriptions s WHERE s.user_id = u.id AND s.status IN ('active', 'trialing'))
		FROM sessions_auth a JOIN users u ON u.id = a.user_id
		WHERE a.token_hash = $1 AND a.kind = $2
		  AND (a.expires_at IS NULL OR a.expires_at > now())
		  AND (a.kind <> 'magic' OR a.used_at IS NULL)`,
		hashToken(raw), kind).Scan(&authID, &u.ID, &u.Email, &u.Handle, &u.Paid)
	if err != nil {
		return nil, err
	}
	if kind == "magic" {
		_, _ = h.db.Exec(ctx, `UPDATE sessions_auth SET used_at = now() WHERE id = $1`, authID)
		_, _ = h.db.Exec(ctx, `UPDATE users SET last_login = now() WHERE id = $1`, u.ID)
	}
	return &u, nil
}

// userFromRequest reads the cookie (browser) or the Bearer token (CLI).
func (h *Hub) userFromRequest(r *http.Request) *User {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		u, err := h.lookupAuth(r.Context(), strings.TrimSpace(auth[7:]), "api")
		if err == nil {
			return u
		}
		return nil
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	u, err := h.lookupAuth(r.Context(), c.Value, "cookie")
	if err != nil {
		return nil
	}
	return u
}

func userOf(r *http.Request) *User {
	u, _ := r.Context().Value(userKey).(*User)
	return u
}

func (h *Hub) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := h.userFromRequest(r)
		if u == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (h *Hub) requireAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := h.userFromRequest(r)
		if u == nil || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid API token; run `deckhand login`"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

// --- users ------------------------------------------------------------------------

var handleClean = regexp.MustCompile(`[^a-z0-9]+`)

// handleFor derives a URL handle from an e-mail: gilles.fauvie@x → gilles-fauvie.
func handleFor(email string) string {
	local := strings.ToLower(strings.SplitN(email, "@", 2)[0])
	hnd := strings.Trim(handleClean.ReplaceAllString(local, "-"), "-")
	if len(hnd) < 3 {
		hnd = "user-" + hnd
	}
	if len(hnd) > 32 {
		hnd = hnd[:32]
	}
	return hnd
}

// findOrCreateUser returns the user for an e-mail, creating it on first login.
func (h *Hub) findOrCreateUser(ctx context.Context, email string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var u User
	err := h.db.QueryRow(ctx, `SELECT id, email, handle FROM users WHERE email = $1`, email).Scan(&u.ID, &u.Email, &u.Handle)
	if err == nil {
		return &u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	base := handleFor(email)
	for i := 0; i < 50; i++ {
		hnd := base
		if i > 0 {
			hnd = fmt.Sprintf("%s-%d", base, i+1)
		}
		err := h.db.QueryRow(ctx, `INSERT INTO users (email, handle) VALUES ($1, $2) ON CONFLICT (handle) DO NOTHING RETURNING id, email, handle`,
			email, hnd).Scan(&u.ID, &u.Email, &u.Handle)
		if err == nil {
			return &u, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return nil, errors.New("cannot allocate a handle")
}

// --- handlers ----------------------------------------------------------------------

func (h *Hub) loginPage(w http.ResponseWriter, r *http.Request) {
	if h.userFromRequest(r) != nil {
		http.Redirect(w, r, "/app", http.StatusFound)
		return
	}
	h.render(w, r, "login.html", map[string]any{"Title": "Sign in"})
}

func (h *Hub) loginPost(w http.ResponseWriter, r *http.Request) {
	addr, err := mail.ParseAddress(strings.TrimSpace(r.FormValue("email")))
	if err != nil {
		h.render(w, r, "login.html", map[string]any{"Title": "Sign in", "Error": "That does not look like an e-mail address."})
		return
	}
	u, err := h.findOrCreateUser(r.Context(), addr.Address)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	raw, err := h.createAuth(r.Context(), u.ID, "magic", "", h.cfg.MagicLinkTTL)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	link := h.cfg.BaseURL + "/auth/callback?token=" + raw
	body := fmt.Sprintf("Hello,\n\nHere is your Deckhand sign-in link (valid %d minutes, single use):\n\n%s\n\nIf you did not ask for it, ignore this e-mail.\n\n— Deckhand, made in Belgium by CODE79\n",
		int(h.cfg.MagicLinkTTL.Minutes()), link)
	if h.cfg.DevLogMagicLinks {
		slog.Info("MAGIC LINK", "email", u.Email, "link", link)
	} else if err := h.sendMail(u.Email, "Your Deckhand sign-in link", body); err != nil {
		slog.Error("send magic link", "err", err)
		h.render(w, r, "login.html", map[string]any{"Title": "Sign in", "Error": "We could not send the e-mail. Please try again in a minute."})
		return
	}
	h.render(w, r, "sent.html", map[string]any{"Title": "Check your inbox", "Email": u.Email})
}

func (h *Hub) authCallback(w http.ResponseWriter, r *http.Request) {
	u, err := h.lookupAuth(r.Context(), r.URL.Query().Get("token"), "magic")
	if err != nil {
		h.render(w, r, "login.html", map[string]any{"Title": "Sign in", "Error": "This link is invalid or has expired. Request a new one."})
		return
	}
	raw, err := h.createAuth(r.Context(), u.ID, "cookie", r.UserAgent(), h.cfg.CookieTTL)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure follows the scheme of BASE_URL
		Name: cookieName, Value: raw, Path: "/", HttpOnly: true, Secure: h.cfg.Secure(),
		SameSite: http.SameSiteLaxMode, MaxAge: int(h.cfg.CookieTTL.Seconds()),
	})
	http.Redirect(w, r, "/app", http.StatusFound)
}

func (h *Hub) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		_, _ = h.db.Exec(r.Context(), `DELETE FROM sessions_auth WHERE token_hash = $1`, hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // deletion cookie, Secure follows BASE_URL
		Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: h.cfg.Secure(), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// newAPIToken creates a CLI token and shows it once.
func (h *Hub) newAPIToken(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	raw, err := h.createAuth(r.Context(), u.ID, "api", "cli "+time.Now().Format("2006-01-02"), 0)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.render(w, r, "token.html", map[string]any{"Title": "API token", "Token": raw, "Hub": h.cfg.BaseURL})
}

func (h *Hub) apiMe(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	plan := "free"
	if u.Paid {
		plan = "pro"
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": u.Email, "handle": u.Handle, "plan": plan})
}
