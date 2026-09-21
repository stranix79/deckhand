package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestNewsletterToken(t *testing.T) {
	now := time.Now()
	tok := newsletterToken(testSecret, now)
	if err := checkNewsletterToken(testSecret, tok, now.Add(5*time.Second)); err != nil {
		t.Fatalf("valid token refused: %v", err)
	}
	if err := checkNewsletterToken(testSecret, tok, now.Add(time.Second)); err == nil {
		t.Fatal("token accepted 1 s after issue, want too-fast refusal")
	}
	if err := checkNewsletterToken(testSecret, tok, now.Add(3*time.Hour)); err == nil {
		t.Fatal("token accepted after 3 h, want expiry")
	}
	if err := checkNewsletterToken("another-secret-another-secret-00", tok, now.Add(5*time.Second)); err == nil {
		t.Fatal("token signed with another secret accepted")
	}
	if err := checkNewsletterToken(testSecret, "garbage", now); err == nil {
		t.Fatal("malformed token accepted")
	}
}

func TestNormalizeEmail(t *testing.T) {
	good := map[string]string{"  Someone@Example.COM ": "someone@example.com", "a.b+c@d.io": "a.b+c@d.io"}
	for in, want := range good {
		got, ok := normalizeEmail(in)
		if !ok || got != want {
			t.Errorf("normalizeEmail(%q) = %q, %v; want %q, true", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "nope", "a@b", "Name <a@b.io>", "a@b.io, c@d.io", strings.Repeat("x", 250) + "@a.io"} {
		if _, ok := normalizeEmail(bad); ok {
			t.Errorf("normalizeEmail(%q) accepted", bad)
		}
	}
}

func TestIPLimiter(t *testing.T) {
	var l ipLimiter
	now := time.Now()
	for i := 0; i < newsletterBurst; i++ {
		if !l.allow("1.2.3.4", now) {
			t.Fatalf("attempt %d refused", i+1)
		}
	}
	if l.allow("1.2.3.4", now) {
		t.Fatal("burst exceeded but allowed")
	}
	if !l.allow("5.6.7.8", now) {
		t.Fatal("other ip refused")
	}
	if !l.allow("1.2.3.4", now.Add(newsletterWindow+time.Second)) {
		t.Fatal("not allowed again after the window")
	}
}

func TestNewsletterSubscribe(t *testing.T) {
	var got map[string]any
	lm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/public/subscription" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"data":true}`))
	}))
	defer lm.Close()

	h := &Hub{cfg: Config{Secret: testSecret, ListmonkURL: lm.URL, NewsletterListFR: "uuid-fr", NewsletterListEN: "uuid-en"}}
	post := func(form url.Values, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/newsletter", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-For", "9.9.9.9, "+ip)
		rec := httptest.NewRecorder()
		h.newsletterSubscribe(rec, req)
		return rec
	}
	token := newsletterToken(testSecret, time.Now().Add(-10*time.Second))

	rec := post(url.Values{"lang": {"fr"}, "email": {"Test@Example.org"}, "t": {token}}, "10.0.0.1")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/?newsletter=ok#fr" {
		t.Fatalf("got %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if got["email"] != "test@example.org" || got["list_uuids"].([]any)[0] != "uuid-fr" {
		t.Fatalf("listmonk payload = %v", got)
	}

	got = nil
	rec = post(url.Values{"lang": {"en"}, "email": {"bot@example.org"}, "t": {token}, "website": {"http://spam"}}, "10.0.0.2")
	if got != nil || rec.Header().Get("Location") != "/?newsletter=ok#en" {
		t.Fatalf("honeypot: subscribed=%v location=%s", got != nil, rec.Header().Get("Location"))
	}

	fresh := newsletterToken(testSecret, time.Now())
	rec = post(url.Values{"lang": {"en"}, "email": {"fast@example.org"}, "t": {fresh}}, "10.0.0.3")
	if got != nil || rec.Header().Get("Location") != "/?newsletter=ok#en" {
		t.Fatalf("instant submission: subscribed=%v location=%s", got != nil, rec.Header().Get("Location"))
	}

	rec = post(url.Values{"lang": {"en"}, "email": {"not an address"}, "t": {token}}, "10.0.0.4")
	if rec.Header().Get("Location") != "/?newsletter=invalid#en" {
		t.Fatalf("invalid address: %s", rec.Header().Get("Location"))
	}

	for i := 0; i < newsletterBurst; i++ {
		post(url.Values{"lang": {"en"}, "email": {"a@example.org"}, "t": {token}}, "10.0.0.5")
	}
	rec = post(url.Values{"lang": {"en"}, "email": {"a@example.org"}, "t": {token}}, "10.0.0.5")
	if rec.Header().Get("Location") != "/?newsletter=wait#en" {
		t.Fatalf("rate limit: %s", rec.Header().Get("Location"))
	}
}
