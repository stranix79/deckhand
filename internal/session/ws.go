package session

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// client is one WebSocket connection.
type client struct {
	role Role
	id   string
	conn *websocket.Conn
	out  chan []byte // buffered; a slow viewer is dropped, not waited for
	done chan struct{}
}

func (c *client) send(raw []byte) {
	select {
	case c.out <- raw:
	default:
		// The buffer is full: this client is too slow. Dropping the frame is
		// safe because every state frame is complete (no deltas); the next one
		// will catch it up. The remote and stage have bigger buffers.
	}
}

func (c *client) close(reason string) {
	if c.conn == nil {
		return // unit tests use fake clients
	}
	_ = c.conn.Close(websocket.StatusNormalClosure, reason)
}

// ServeWS upgrades the request and runs the connection until it ends.
// Query parameters: role=stage|remote|viewer (default viewer), t=token.
// Options: AcceptOrigins for cross-origin remotes (hub).
func (s *Session) ServeWS(w http.ResponseWriter, r *http.Request, acceptOrigins []string) {
	role := Role(r.URL.Query().Get("role"))
	if role == "" {
		role = RoleViewer
	}
	if role != RoleStage && role != RoleRemote && role != RoleViewer {
		http.Error(w, "unknown role", http.StatusBadRequest)
		return
	}
	needToken := role == RoleRemote || (role == RoleStage && s.StageNeedsToken)
	if needToken && subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("t")), []byte(s.Token)) != 1 {
		http.Error(w, "invalid token", http.StatusForbidden)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: acceptOrigins})
	if err != nil {
		slog.Debug("ws accept", "err", err)
		return
	}
	buf := 64
	if role == RoleViewer {
		buf = 16
	}
	c := &client{role: role, id: NewToken()[:12], conn: conn, out: make(chan []byte, buf), done: make(chan struct{})}
	s.attach(c)
	defer s.detach(c)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Writer goroutine: one writer per connection, as the library requires.
	go func() {
		defer close(c.done)
		for {
			select {
			case <-ctx.Done():
				return
			case raw := <-c.out:
				wctx, wcancel := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Write(wctx, websocket.MessageText, raw)
				wcancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// Reader loop. Viewers send nothing but we still read to notice closes
	// (and to answer pings, which the library does for us).
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			break
		}
		if err := s.apply(c, raw); err != nil {
			if errors.Is(err, errForbidden) {
				c.close("viewers cannot send")
				break
			}
			slog.Debug("ws op", "role", role, "err", err)
		}
	}
	cancel()
	<-c.done
	c.close("bye")
}

func (s *Session) attach(c *client) {
	s.mu.Lock()
	s.clients[c] = struct{}{}
	if c.role == RoleStage {
		s.stage = c
	}
	c.send(s.deckMessage(c.role))
	c.send(mustJSON(map[string]any{"op": "state", "state": s.state}))
	c.send(mustJSON(map[string]any{"op": "viewers", "count": s.countViewersLocked()}))
	slide := s.state.Slide
	s.mu.Unlock()
	if s.OnEvent != nil && c.role == RoleViewer {
		s.OnEvent(Event{At: time.Now(), Code: s.Code, Role: c.role, ViewerID: c.id, Kind: "join", Slide: slide})
	}
}

func (s *Session) detach(c *client) {
	s.mu.Lock()
	delete(s.clients, c)
	if s.stage == c {
		s.stage = nil
		for other := range s.clients {
			if other.role == RoleStage {
				s.stage = other
				break
			}
		}
		// A pending question to a vanished stage is answered "not handled".
		if s.pending != nil && s.stage == nil {
			a := s.pending
			a.timer.Stop()
			s.pending = nil
			s.stepSlideLocked(a.dir)
		}
	}
	slide := s.state.Slide
	s.mu.Unlock()
	if s.OnEvent != nil && c.role == RoleViewer {
		s.OnEvent(Event{At: time.Now(), Code: s.Code, Role: c.role, ViewerID: c.id, Kind: "leave", Slide: slide})
	}
}

// RoleFromPath is a helper for routers: "/s/" → stage, "/r/" → remote, else viewer.
func RoleFromPath(p string) Role {
	switch {
	case strings.HasPrefix(p, "/s/"):
		return RoleStage
	case strings.HasPrefix(p, "/r/"):
		return RoleRemote
	default:
		return RoleViewer
	}
}
