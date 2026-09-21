package hub

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The landing's newsletter form is plain HTML and posts here. Nothing
// third-party is loaded in the browser; the anti-spam is server-side:
//   - a honeypot field that bots fill in and humans never see,
//   - a signed timestamp issued with the page, refused when the form comes
//     back in under newsletterMinDelay (bots post instantly) or after
//     newsletterMaxAge,
//   - a budget of newsletterBurst sign-ups per hour per client IP,
//   - a syntactic check of the address.
//
// On success the address is subscribed through Listmonk's public API
// (single opt-in lists), server to server, so Listmonk's admin API never has
// to be reachable from the internet. Refusals are answered like successes:
// a bot learns nothing, a human never hits them.
const (
	newsletterMinDelay    = 3 * time.Second
	newsletterMaxAge      = 2 * time.Hour
	newsletterBurst       = 5
	newsletterWindow      = time.Hour
	newsletterPlaceholder = "{{NEWSLETTER_TOKEN}}"
)

// ipLimiter is a fixed-window counter per IP, in memory (one hub process).
type ipLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

var newsletterLimit ipLimiter

func (l *ipLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string][]time.Time{}
	}
	if len(l.hits) > 10000 {
		for k, v := range l.hits {
			if len(v) == 0 || now.Sub(v[len(v)-1]) > newsletterWindow {
				delete(l.hits, k)
			}
		}
	}
	keep := l.hits[ip][:0]
	for _, t := range l.hits[ip] {
		if now.Sub(t) < newsletterWindow {
			keep = append(keep, t)
		}
	}
	if len(keep) >= newsletterBurst {
		l.hits[ip] = keep
		return false
	}
	l.hits[ip] = append(keep, now)
	return true
}

// newsletterToken is "<unix>.<hmac>", issued when the landing is served.
func newsletterToken(secret string, now time.Time) string {
	ts := strconv.FormatInt(now.Unix(), 10)
	return ts + "." + newsletterSign(secret, ts)
}

func newsletterSign(secret, ts string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("newsletter:" + ts))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
}

func checkNewsletterToken(secret, token string, now time.Time) error {
	ts, sig, ok := strings.Cut(token, ".")
	if !ok {
		return errors.New("malformed token")
	}
	if !hmac.Equal([]byte(newsletterSign(secret, ts)), []byte(sig)) {
		return errors.New("bad token signature")
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errors.New("bad token timestamp")
	}
	age := now.Sub(time.Unix(unix, 0))
	if age < newsletterMinDelay {
		return errors.New("form submitted too fast")
	}
	if age > newsletterMaxAge {
		return errors.New("token expired")
	}
	return nil
}

// normalizeEmail accepts one bare address (no display name) with a dotted
// domain and returns it lower-cased.
func normalizeEmail(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 254 {
		return "", false
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s {
		return "", false
	}
	if at := strings.LastIndex(s, "@"); at < 1 || !strings.Contains(s[at+1:], ".") {
		return "", false
	}
	return strings.ToLower(s), true
}

// clientIP is the last X-Forwarded-For entry (the one nginx appends, so it
// cannot be spoofed by the client) or the connection address.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *Hub) newsletterSubscribe(w http.ResponseWriter, r *http.Request) {
	lang := "en"
	if r.FormValue("lang") == "fr" {
		lang = "fr"
	}
	back := func(status string) {
		http.Redirect(w, r, "/?newsletter="+status+"#"+lang, http.StatusSeeOther)
	}
	if !h.cfg.NewsletterEnabled() {
		back("off")
		return
	}
	if r.FormValue("website") != "" { // honeypot
		slog.Info("newsletter refused", "reason", "honeypot")
		back("ok")
		return
	}
	if err := checkNewsletterToken(h.cfg.Secret, r.FormValue("t"), time.Now()); err != nil {
		slog.Info("newsletter refused", "reason", err.Error())
		back("ok")
		return
	}
	if !newsletterLimit.allow(clientIP(r), time.Now()) {
		back("wait")
		return
	}
	email, ok := normalizeEmail(r.FormValue("email"))
	if !ok {
		back("invalid")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if rs := []rune(name); len(rs) > 100 {
		name = string(rs[:100])
	}
	list := h.cfg.NewsletterListEN
	if lang == "fr" {
		list = h.cfg.NewsletterListFR
	}
	if err := listmonkSubscribe(r.Context(), h.cfg.ListmonkURL, email, name, list); err != nil {
		slog.Error("newsletter subscribe", "err", err)
		back("error")
		return
	}
	slog.Info("newsletter subscribed", "lang", lang)
	back("ok")
}

// listmonkSubscribe calls Listmonk's unauthenticated public endpoint. An
// address already on the list counts as success.
func listmonkSubscribe(ctx context.Context, base, email, name, listUUID string) error {
	body, err := json.Marshal(map[string]any{
		"email": email, "name": name, "list_uuids": []string{listUUID},
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(base, "/")+"/api/public/subscription", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 == 2 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if strings.Contains(strings.ToLower(string(b)), "exist") {
		return nil
	}
	return fmt.Errorf("listmonk %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
}
