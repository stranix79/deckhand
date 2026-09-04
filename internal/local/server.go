// Package local is `deckhand present`: one session, one HTTP server on the
// LAN, QR codes in the terminal.
package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/stranix79/deckhand/internal/deck"
	"github.com/stranix79/deckhand/internal/session"
	"github.com/stranix79/deckhand/internal/ui"
)

// Options of a local presentation.
type Options struct {
	Port  int    // preferred port, 7777 by default
	IP    string // force the LAN IP shown in the URLs
	Open  bool   // open the stage in the default browser
	NoLAN bool   // listen on 127.0.0.1 only
	Out   io.Writer
	Color bool

	// Hub relay (milestone 4): when set, the deck is pushed to the hub and
	// every state change is relayed there.
	Hub      string
	HubToken string
	HubSlug  string
}

// URLs of a running presentation.
type URLs struct {
	Stage, Remote, Viewer string
}

// Presentation is a running local server.
type Presentation struct {
	Session *session.Session
	URLs    URLs
	Addr    string
	server  *http.Server
	ln      net.Listener
}

// Start loads nothing: it takes a validated deck, binds the port and returns
// as soon as the server is listening. Serve blocks until ctx ends.
func Start(ctx context.Context, d *deck.Deck, o Options) (*Presentation, error) {
	host := ""
	shownIP := "127.0.0.1"
	if o.NoLAN {
		host = "127.0.0.1"
	} else {
		ip, err := LANIP(o.IP)
		if err != nil {
			return nil, err
		}
		shownIP = ip
	}
	if o.Port == 0 {
		o.Port = 7777
	}
	port, err := FreePort(host, o.Port)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, err
	}
	port = ln.Addr().(*net.TCPAddr).Port // the real one (port 0 = random)

	s := session.New(d)
	base := fmt.Sprintf("http://%s:%d", shownIP, port)
	urls := URLs{
		Stage:  base + "/s/" + s.Code,
		Remote: base + "/r/" + s.Code + "?t=" + s.Token,
		Viewer: base + "/v/" + s.Code,
	}
	s.ViewerURL = urls.Viewer

	mgr := session.NewManager()
	mgr.Add(s)
	u := &ui.Server{Lookup: mgr.Get}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5, "text/html", "text/css", "text/javascript", "application/json"))
	u.AppRoutes(r)
	u.DeckRoutes(r)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/s/"+s.Code, http.StatusFound)
	})
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })

	p := &Presentation{
		Session: s,
		URLs:    urls,
		Addr:    ln.Addr().String(),
		ln:      ln,
		server: &http.Server{
			Handler:           r,
			ReadHeaderTimeout: 10 * time.Second,
			BaseContext:       func(net.Listener) context.Context { return ctx },
		},
	}
	return p, nil
}

// Serve runs the server until ctx is cancelled, then shuts it down.
func (p *Presentation) Serve(ctx context.Context) error {
	errc := make(chan error, 1)
	go func() { errc <- p.server.Serve(p.ln) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		p.Session.Close()
		if err := p.server.Shutdown(shutdownCtx); err != nil {
			// A browser holding a half-open connection must not keep us alive.
			_ = p.server.Close()
		}
		return nil
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Banner prints the URLs and the two QR codes (remote and audience).
func (p *Presentation) Banner(w io.Writer, title string, nslides int, color bool) {
	c := func(code, s string) string {
		if !color {
			return s
		}
		return "\033[" + code + "m" + s + "\033[0m"
	}
	fmt.Fprintf(w, "\n  %s  %s\n", c("1", title), c("2", fmt.Sprintf("%d slides", nslides)))
	fmt.Fprintf(w, "  %s   %s\n", c("2", "stage  "), c("1;36", p.URLs.Stage))
	fmt.Fprintf(w, "  %s   %s\n", c("2", "remote "), c("1;33", p.URLs.Remote))
	fmt.Fprintf(w, "  %s   %s\n\n", c("2", "public "), c("1;32", p.URLs.Viewer))
	remote := strings.Split(ASCIIQR(p.URLs.Remote), "\n")
	viewer := strings.Split(ASCIIQR(p.URLs.Viewer), "\n")
	width := 0
	for _, l := range remote {
		if n := len([]rune(l)); n > width {
			width = n
		}
	}
	fmt.Fprintf(w, "  %-*s   %s\n", width, c("1;33", "REMOTE (you)")+strings.Repeat(" ", width-12), c("1;32", "AUDIENCE"))
	for i := 0; i < len(remote) || i < len(viewer); i++ {
		l, r := "", ""
		if i < len(remote) {
			l = remote[i]
		}
		if i < len(viewer) {
			r = viewer[i]
		}
		fmt.Fprintf(w, "  %s%s   %s\n", l, strings.Repeat(" ", width-len([]rune(l))), r)
	}
	fmt.Fprintf(w, "\n  %s\n\n", c("2", "keys on the stage: → ← space · f fullscreen · b black · q QR · ctrl-c to stop"))
}

// OpenBrowser opens url with the OS default browser. The URL is ours (built
// from the LAN IP and the session code), not user input.
func OpenBrowser(url string) error { //nolint:gosec // see above
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) //nolint:gosec // our own URL
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec // our own URL
	default:
		cmd = exec.Command("xdg-open", url) //nolint:gosec // our own URL
	}
	return cmd.Start()
}
