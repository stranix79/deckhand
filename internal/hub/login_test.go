package hub

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The refusal paths of the sign-in form must answer like a success and
// return before touching the database or sending anything (h has no DB and
// no SMTP here: reaching them would panic or fail).
func TestLoginPostRefusesBots(t *testing.T) {
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	h := &Hub{cfg: Config{Secret: testSecret, BaseURL: "https://example.test"}, tmpl: tmpl}
	post := func(form url.Values, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-For", ip)
		rec := httptest.NewRecorder()
		h.loginPost(rec, req)
		return rec
	}
	old := newsletterToken(testSecret, time.Now().Add(-10*time.Second))

	rec := post(url.Values{"email": {"victim@example.org"}, "t": {old}, "website": {"x"}}, "10.1.0.1")
	if !strings.Contains(rec.Body.String(), "victim@example.org") {
		t.Fatalf("honeypot: want the fake 'sent' page, got %d", rec.Code)
	}

	rec = post(url.Values{"email": {"victim@example.org"}, "t": {newsletterToken(testSecret, time.Now())}}, "10.1.0.2")
	if !strings.Contains(rec.Body.String(), "victim@example.org") {
		t.Fatalf("instant post: want the fake 'sent' page, got %d", rec.Code)
	}

	rec = post(url.Values{"email": {"victim@example.org"}}, "10.1.0.3")
	if !strings.Contains(rec.Body.String(), "victim@example.org") {
		t.Fatalf("missing token: want the fake 'sent' page, got %d", rec.Code)
	}

	now := time.Now()
	for i := 0; i < loginBurstPerEmail; i++ {
		loginLimitEmail.allowN("target@example.org", now, loginBurstPerEmail)
	}
	rec = post(url.Values{"email": {"Target@Example.org"}, "t": {old}}, "10.1.0.4")
	if !strings.Contains(rec.Body.String(), "Target@Example.org") {
		t.Fatalf("per-address limit: want the fake 'sent' page, got %d", rec.Code)
	}
}
