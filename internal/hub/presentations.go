package hub

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/stranix79/deckhand/internal/deck"
	"github.com/stranix79/deckhand/internal/session"
)

// PresRow is a presentations row.
type PresRow struct {
	ID, DeckID, UserID, Code, Token, Mode string
	State                                 json.RawMessage
	StartedAt                             time.Time
	EndedAt                               *time.Time
}

const presCols = `id, deck_id, user_id, code, token, mode, state, started_at, ended_at`

func scanPres(row pgx.Row) (*PresRow, error) {
	var p PresRow
	if err := row.Scan(&p.ID, &p.DeckID, &p.UserID, &p.Code, &p.Token, &p.Mode, &p.State, &p.StartedAt, &p.EndedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

// startPresentation creates a live presentation of a deck and its in-memory
// session. mode: "hosted" (driven from /r on the hub), "relay" (driven by
// a local `deckhand present --hub`).
func (h *Hub) startPresentation(ctx context.Context, d *DeckRow, u *User, mode string) (*PresRow, *session.Session, error) {
	code, token := session.NewCode(), session.NewToken()
	row, err := scanPres(h.db.QueryRow(ctx, `INSERT INTO presentations (deck_id, user_id, code, token, mode) VALUES ($1, $2, $3, $4, $5) RETURNING `+presCols,
		d.ID, u.ID, code, token, mode))
	if err != nil {
		return nil, nil, err
	}
	s, err := h.newSession(row, d, u.Paid)
	if err != nil {
		return nil, nil, err
	}
	h.sessions.Add(s)
	return row, s, nil
}

// newSession loads the deck files and builds the session for a presentation.
func (h *Hub) newSession(p *PresRow, d *DeckRow, paid bool) (*session.Session, error) {
	dk, err := deck.Load(d.Path)
	if err != nil {
		return nil, err
	}
	s := session.New(dk)
	s.Code, s.Token = p.Code, p.Token
	s.FilesBase = h.cfg.DeckOrigin + "/deck/" + p.Code
	s.ViewerURL = h.cfg.BaseURL + "/v/" + p.Code
	s.StageNeedsToken = true
	if !paid {
		s.MaxViewers = h.cfg.FreeMaxViewers
	}
	if len(p.State) > 2 {
		var st session.State
		if json.Unmarshal(p.State, &st) == nil {
			s.SetState(st)
		}
	}
	presID := p.ID
	if p.Mode != "permalink" {
		s.OnEvent = func(ev session.Event) {
			select {
			case h.events <- eventRow{presID: presID, ev: ev}:
			default: // statistics are best effort
			}
		}
		s.OnState = func(st session.State) {
			raw, _ := json.Marshal(st)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, _ = h.db.Exec(ctx, `UPDATE presentations SET state = $1 WHERE id = $2`, raw, presID)
			}()
		}
	}
	return s, nil
}

// Lookup resolves a session code: memory first, then the database (a
// presentation started by another process, or a permalink being opened).
func (h *Hub) Lookup(code string) *session.Session {
	code = strings.ToUpper(code)
	if s := h.sessions.Get(code); s != nil {
		return s
	}
	h.loadMu.Lock()
	defer h.loadMu.Unlock()
	if s := h.sessions.Get(code); s != nil {
		return s
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := scanPres(h.db.QueryRow(ctx, `SELECT `+presCols+` FROM presentations WHERE code = $1 AND ended_at IS NULL`, code))
	if err != nil {
		return nil
	}
	d, err := h.getDeck(ctx, p.DeckID)
	if err != nil {
		return nil
	}
	if p.Mode == "permalink" && d.Expired() {
		return nil
	}
	var paid bool
	_ = h.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM subscriptions WHERE user_id = $1 AND status IN ('active', 'trialing'))`, p.UserID).Scan(&paid)
	s, err := h.newSession(p, d, paid)
	if err != nil {
		slog.Error("load session", "code", code, "err", err)
		return nil
	}
	h.sessions.Add(s)
	return s
}

// permalinkSession returns the code of the always-on, viewer-only session
// of a deck (/d/{user}/{slug}), creating its presentation row on first use.
func (h *Hub) permalinkSession(ctx context.Context, d *DeckRow) (string, error) {
	if d.PermalinkCode == "" {
		return "", errors.New("deck without permalink code")
	}
	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM presentations WHERE code = $1 AND ended_at IS NULL)`, d.PermalinkCode).Scan(&exists); err != nil {
		return "", err
	}
	if !exists {
		if _, err := h.db.Exec(ctx, `INSERT INTO presentations (deck_id, user_id, code, token, mode) VALUES ($1, $2, $3, $4, 'permalink') ON CONFLICT (code) DO UPDATE SET ended_at = NULL, deck_id = EXCLUDED.deck_id`,
			d.ID, d.UserID, d.PermalinkCode, session.NewToken()); err != nil {
			return "", err
		}
	}
	return d.PermalinkCode, nil
}

// endPresentation closes a live presentation.
func (h *Hub) endPresentation(ctx context.Context, id string) error {
	var code string
	err := h.db.QueryRow(ctx, `UPDATE presentations SET ended_at = now() WHERE id = $1 AND ended_at IS NULL RETURNING code`, id).Scan(&code)
	if err != nil {
		return err
	}
	h.sessions.Remove(code)
	return nil
}

func (h *Hub) getPresentation(ctx context.Context, id string) (*PresRow, error) {
	return scanPres(h.db.QueryRow(ctx, `SELECT `+presCols+` FROM presentations WHERE id = $1`, id))
}

// --- relay: `deckhand present --hub` ----------------------------------------------

type relayState struct {
	mu   sync.Mutex
	last map[string]time.Time // presentation code → last frame from the CLI
}

// relayWS receives state frames from a local presentation and mirrors them
// into the hosted session, so remote viewers follow the LAN presentation.
//
//	CLI → hub: {op:"state", state:{...}, ts:<unix ms>} | {op:"end"}
//	hub → CLI: {op:"hello", viewerUrl} then {op:"viewers", count} every 2 s
func (h *Hub) relayWS(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(chi.URLParam(r, "code"))
	s := h.Lookup(code)
	if s == nil {
		http.Error(w, "no such presentation", http.StatusNotFound)
		return
	}
	if r.URL.Query().Get("t") != s.Token {
		http.Error(w, "invalid token", http.StatusForbidden)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true}) // CLI, no browser origin
	if err != nil {
		return
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "bye") }()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	h.metrics.relays.Inc()
	defer h.metrics.relays.Dec()

	send := func(v any) {
		raw, _ := json.Marshal(v)
		wctx, wcancel := context.WithTimeout(ctx, 5*time.Second)
		defer wcancel()
		_ = conn.Write(wctx, websocket.MessageText, raw)
	}
	send(map[string]any{"op": "hello", "viewerUrl": s.ViewerURL, "code": code})
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		last := -1
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n := s.Viewers(); n != last {
					last = n
					send(map[string]any{"op": "viewers", "count": n})
				}
			}
		}
	}()

	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var f struct {
			Op    string        `json:"op"`
			State session.State `json:"state"`
			TS    int64         `json:"ts"`
		}
		if json.Unmarshal(raw, &f) != nil {
			continue
		}
		switch f.Op {
		case "state":
			start := time.Now()
			s.SetState(f.State)
			h.metrics.relayLatency.Observe(time.Since(start).Seconds())
			h.relay.mu.Lock()
			h.relay.last[code] = time.Now()
			h.relay.mu.Unlock()
		case "end":
			var id string
			if h.db.QueryRow(ctx, `SELECT id FROM presentations WHERE code = $1`, code).Scan(&id) == nil {
				_ = h.endPresentation(context.Background(), id)
			}
			return
		}
	}
}

// janitor ends relay presentations whose CLI went silent for 15 minutes and
// drops idle permalink sessions from memory (they reload on demand).
func (h *Hub) janitor(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.relay.mu.Lock()
			var stale []string
			for code, last := range h.relay.last {
				if time.Since(last) > 15*time.Minute {
					stale = append(stale, code)
					delete(h.relay.last, code)
				}
			}
			h.relay.mu.Unlock()
			for _, code := range stale {
				var id string
				if h.db.QueryRow(ctx, `SELECT id FROM presentations WHERE code = $1 AND mode = 'relay' AND ended_at IS NULL`, code).Scan(&id) == nil {
					_ = h.endPresentation(ctx, id)
				}
			}
			h.sessions.Each(func(s *session.Session) {
				if s.Viewers() == 0 && time.Since(s.CreatedAt) > time.Hour && !h.hasNonViewer(s) {
					// Nobody is connected: free the memory, Lookup reloads it.
					h.sessions.Remove(s.Code)
				}
			})
		}
	}
}

func (h *Hub) hasNonViewer(s *session.Session) bool { return s.Clients() > s.Viewers() }

// --- viewer events (statistics) ------------------------------------------------------

type eventRow struct {
	presID string
	ev     session.Event
}

func (h *Hub) eventWriter(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-h.events:
			wctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, err := h.db.Exec(wctx, `INSERT INTO viewers_events (presentation_id, viewer_id, kind, slide, at) VALUES ($1, $2, $3, $4, $5)`,
				e.presID, e.ev.ViewerID, e.ev.Kind, e.ev.Slide, e.ev.At)
			cancel()
			if err != nil {
				slog.Debug("viewer event", "err", err)
			}
		}
	}
}
