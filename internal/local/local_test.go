package local

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stranix79/deckhand/internal/deck"
)

func TestStartAndBanner(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"1.html", "2.html"} {
		if err := os.WriteFile(filepath.Join(root, n), []byte("<html></html>"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	d, err := deck.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := Start(ctx, d, Options{NoLAN: true, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- p.Serve(ctx) }()

	if !strings.HasPrefix(p.URLs.Stage, "http://127.0.0.1:") || !strings.Contains(p.URLs.Remote, "?t="+p.Session.Token) {
		t.Fatalf("urls: %+v", p.URLs)
	}
	resp, err := http.Get(p.URLs.Stage)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("stage: %v %v", err, resp)
	}
	_ = resp.Body.Close()
	resp, err = http.Get("http://" + p.Addr + "/")
	if err != nil || resp.StatusCode != 200 { // followed redirect to the stage
		t.Fatalf("root redirect: %v", err)
	}
	_ = resp.Body.Close()

	var sb strings.Builder
	p.Banner(&sb, "T", 2, false)
	if !strings.Contains(sb.String(), "REMOTE") || !strings.Contains(sb.String(), "█") {
		t.Fatalf("banner without QR:\n%s", sb.String())
	}
	http.DefaultClient.CloseIdleConnections()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestFreePortAndLANIP(t *testing.T) {
	p, err := FreePort("127.0.0.1", 7777)
	if err != nil || p < 7777 {
		t.Fatalf("FreePort: %d %v", p, err)
	}
	if _, err := LANIP("not-an-ip"); err == nil {
		t.Fatal("bad --ip accepted")
	}
	if ip, err := LANIP("10.1.2.3"); err != nil || ip != "10.1.2.3" {
		t.Fatal("forced ip")
	}
	if !skipInterface("docker0") || !skipInterface("utun3") || skipInterface("en0") {
		t.Fatal("interface filter")
	}
}
