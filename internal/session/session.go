// Package session holds the live state of one presentation and fans it out to
// every connected screen over WebSocket.
//
// A session is a loaded deck plus a State (current slide, fragment, laser
// pointer, start time). Three roles connect to it (brief §4):
//
//   - stage:  the projector; also answers the fragment question (see ask below)
//   - remote: the presenter's phone; must present the session token
//   - viewer: anybody; may only listen
//
// The package has no HTTP knowledge beyond the WebSocket handshake: the local
// server and the hub both mount ServeWS on /ws/{code}.
package session

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/stranix79/deckhand/internal/deck"
)

// Role of a WebSocket client.
type Role string

// The three roles.
const (
	RoleStage  Role = "stage"
	RoleRemote Role = "remote"
	RoleViewer Role = "viewer"
)

// Pointer is the laser position in normalised slide coordinates (0..1).
type Pointer struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// State is what every screen renders. It is sent in full on every change.
type State struct {
	Slide     int        `json:"slide"`
	Fragment  int        `json:"fragment"`
	Pointer   *Pointer   `json:"pointer"`
	StartedAt *time.Time `json:"startedAt"` // first "next"; nil until then
	Black     bool       `json:"black"`     // stage shows a black screen
}

// Event is emitted for statistics (hub) and logging. Local mode ignores it.
type Event struct {
	At       time.Time
	Code     string
	Role     Role
	ViewerID string // stable per connection, anonymous
	Kind     string // "join", "leave", "slide"
	Slide    int
}

// Session is one live presentation.
type Session struct {
	Code      string
	Token     string // secret required by the remote (and by the stage when StageNeedsToken)
	Deck      *deck.Deck
	FilesBase string // URL prefix of the deck files, e.g. "/deck/K7RTQP" or "https://decks.x/deck/K7RTQP"
	ViewerURL string // public link, shown as a QR by the remote and the stage
	CreatedAt time.Time

	// StageNeedsToken makes the stage present the token too. Off on the LAN
	// (the brief lets the presenter drive from the stage keyboard), on for
	// hosted presentations where /s/{code} is guessable.
	StageNeedsToken bool

	// OnEvent, when set, receives join/leave/slide events (hub statistics).
	OnEvent func(Event)

	// OnState, when set, is called after every state change (relay to a hub,
	// persistence). Called synchronously under the session lock: keep it fast
	// (hand off to a channel).
	OnState func(State)

	// MaxViewers, when > 0, refuses viewers beyond that count (close code 4429).
	MaxViewers int

	mu       sync.Mutex
	state    State
	clients  map[*client]struct{}
	stage    *client // most recent stage, receives the fragment questions
	pending  *ask    // outstanding fragment question, if any
	seq      int
	viewers  int // last broadcast viewer count
	closed   bool
	closeCh  chan struct{}
	askDelay time.Duration
}

// ask is a fragment question sent to the stage: "would the slide handle
// next/prev itself?". The answer, or its absence after askDelay, decides
// between moving a fragment and moving a slide.
type ask struct {
	seq   int
	dir   int // +1 or -1
	timer *time.Timer
}

// New creates a session for a validated deck. Code and Token are generated.
func New(d *deck.Deck) *Session {
	s := &Session{
		Code:      NewCode(),
		Token:     NewToken(),
		Deck:      d,
		CreatedAt: time.Now(),
		clients:   map[*client]struct{}{},
		closeCh:   make(chan struct{}),
		askDelay:  400 * time.Millisecond, // 150 ms in the slide + network
	}
	s.FilesBase = "/deck/" + s.Code
	go s.viewerTicker()
	return s
}

// State returns a copy of the current state.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// SetState replaces the state (used by the hub relay, which mirrors a local
// session) and broadcasts it.
func (s *Session) SetState(st State) {
	s.mu.Lock()
	s.state = st
	s.broadcastStateLocked()
	s.mu.Unlock()
}

// Clients is the number of connected clients of any role.
func (s *Session) Clients() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

// Viewers is the number of connected viewers.
func (s *Session) Viewers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.countViewersLocked()
}

// Close disconnects everybody and stops the tickers.
func (s *Session) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.closeCh)
	if s.pending != nil {
		s.pending.timer.Stop()
		s.pending = nil
	}
	clients := make([]*client, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.Unlock()
	for _, c := range clients {
		c.close("session closed")
	}
}

// --- operations (client → server) ------------------------------------------

// op is the wire format of a client message. One struct for every op keeps
// decoding trivial; unused fields stay zero.
type op struct {
	Op       string   `json:"op"`
	Slide    *int     `json:"slide,omitempty"`
	X        *float64 `json:"x,omitempty"`
	Y        *float64 `json:"y,omitempty"`
	Seq      int      `json:"seq,omitempty"`
	Handled  bool     `json:"handled,omitempty"`
	Fragment *int     `json:"fragment,omitempty"`
	Seconds  *int     `json:"seconds,omitempty"`
}

// errForbidden is returned when a role sends an op it may not send.
var errForbidden = errors.New("forbidden")

// apply runs one op from a client. Viewers may not send anything.
func (s *Session) apply(c *client, raw []byte) error {
	var o op
	if err := json.Unmarshal(raw, &o); err != nil {
		return err
	}
	if c.role == RoleViewer {
		return errForbidden
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	switch o.Op {
	case "next":
		s.moveLocked(+1)
	case "prev":
		s.moveLocked(-1)
	case "goto":
		if o.Slide == nil {
			return errors.New("goto without slide")
		}
		s.gotoLocked(*o.Slide, 0)
	case "fragment":
		// The stage reports the fragment index it is showing (after a keyboard
		// action inside the stage itself, for instance).
		if o.Fragment != nil && *o.Fragment >= 0 {
			s.state.Fragment = *o.Fragment
			s.broadcastStateLocked()
		}
	case "answer":
		s.answerLocked(o.Seq, o.Handled)
	case "pointer":
		if o.X == nil || o.Y == nil {
			s.state.Pointer = nil
		} else {
			s.state.Pointer = &Pointer{X: clamp01(*o.X), Y: clamp01(*o.Y)}
		}
		s.broadcastStateLocked()
	case "black":
		s.state.Black = !s.state.Black
		s.broadcastStateLocked()
	case "qr":
		// Transient: the stage shows the audience QR for a few seconds
		// (default 15). seconds:0 hides it. Everybody gets the frame so the
		// remote can flip its button.
		secs := 15
		if o.Seconds != nil && *o.Seconds >= 0 && *o.Seconds <= 120 {
			secs = *o.Seconds
		}
		s.broadcastLocked(mustJSON(map[string]any{"op": "qr", "seconds": secs}), nil)
	case "reset":
		s.state.StartedAt = nil
		s.broadcastStateLocked()
	default:
		return errors.New("unknown op " + o.Op)
	}
	return nil
}

// moveLocked handles next/prev. If a stage is connected it is asked first
// (fragments); otherwise the slide changes immediately.
func (s *Session) moveLocked(dir int) {
	s.startClockLocked()
	if s.stage == nil || s.pending != nil {
		s.stepSlideLocked(dir)
		return
	}
	s.seq++
	a := &ask{seq: s.seq, dir: dir}
	a.timer = time.AfterFunc(s.askDelay, func() { s.answer(a.seq, false) })
	s.pending = a
	s.stage.send(mustJSON(map[string]any{"op": "ask", "seq": a.seq, "dir": dirName(dir)}))
}

// answer is the timer/stage callback for a fragment question.
func (s *Session) answer(seq int, handled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answerLocked(seq, handled)
}

func (s *Session) answerLocked(seq int, handled bool) {
	a := s.pending
	if a == nil || a.seq != seq {
		return // late or duplicate answer
	}
	a.timer.Stop()
	s.pending = nil
	if handled {
		s.state.Fragment += a.dir
		if s.state.Fragment < 0 {
			s.state.Fragment = 0
		}
		s.broadcastStateLocked()
		return
	}
	s.stepSlideLocked(a.dir)
}

func (s *Session) stepSlideLocked(dir int) {
	s.gotoLocked(s.state.Slide+dir, 0)
}

func (s *Session) gotoLocked(slide, fragment int) {
	n := len(s.Deck.Slides)
	if slide < 0 {
		slide = 0
	}
	if slide > n-1 {
		slide = n - 1
	}
	changed := slide != s.state.Slide
	s.state.Slide = slide
	s.state.Fragment = fragment
	s.state.Pointer = nil
	s.broadcastStateLocked()
	if changed && s.OnEvent != nil {
		s.OnEvent(Event{At: time.Now(), Code: s.Code, Kind: "slide", Slide: slide})
	}
}

func (s *Session) startClockLocked() {
	if s.state.StartedAt == nil {
		now := time.Now()
		s.state.StartedAt = &now
	}
}

// --- broadcast --------------------------------------------------------------

func (s *Session) broadcastStateLocked() {
	s.broadcastLocked(mustJSON(map[string]any{"op": "state", "state": s.state}), nil)
	if s.OnState != nil {
		s.OnState(s.state)
	}
}

// broadcastLocked sends raw to every client (or only to those for which
// filter returns true).
func (s *Session) broadcastLocked(raw []byte, filter func(*client) bool) {
	for c := range s.clients {
		if filter == nil || filter(c) {
			c.send(raw)
		}
	}
}

// viewerTicker broadcasts the viewer count every 2 s when it changed.
func (s *Session) viewerTicker() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.closeCh:
			return
		case <-t.C:
			s.mu.Lock()
			n := s.countViewersLocked()
			if n != s.viewers {
				s.viewers = n
				s.broadcastLocked(mustJSON(map[string]any{"op": "viewers", "count": n}), nil)
			}
			s.mu.Unlock()
		}
	}
}

func (s *Session) countViewersLocked() int {
	n := 0
	for c := range s.clients {
		if c.role == RoleViewer {
			n++
		}
	}
	return n
}

// deckMessage is the "deck" frame sent once per connection. Notes go to the
// remote; viewers only get the notes flagged public.
func (s *Session) deckMessage(role Role) []byte {
	type slide struct {
		Index  int    `json:"index"`
		URL    string `json:"url"`
		Notes  string `json:"notes,omitempty"`
		Public bool   `json:"public,omitempty"`
	}
	slides := make([]slide, 0, len(s.Deck.Slides))
	for _, sl := range s.Deck.Slides {
		x := slide{Index: sl.Index, URL: s.FilesBase + "/" + sl.File, Public: sl.Public}
		if role == RoleRemote || role == RoleStage || sl.Public {
			x.Notes = sl.Notes
		}
		slides = append(slides, x)
	}
	msg := map[string]any{
		"op": "deck",
		"deck": map[string]any{
			"title":  s.Deck.Title,
			"ratio":  s.Deck.Ratio,
			"width":  s.Deck.Width,
			"height": s.Deck.Height,
			"slides": slides,
		},
		"code": s.Code,
	}
	if role != RoleViewer {
		msg["viewerUrl"] = s.ViewerURL
	}
	return mustJSON(msg)
}

// --- helpers -----------------------------------------------------------------

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func dirName(d int) string {
	if d < 0 {
		return "prev"
	}
	return "next"
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // only static structures are marshalled here
	}
	return b
}

// Manager keeps the live sessions of a process, by code.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewManager returns an empty manager.
func NewManager() *Manager { return &Manager{sessions: map[string]*Session{}} }

// Add registers a session.
func (m *Manager) Add(s *Session) {
	m.mu.Lock()
	m.sessions[s.Code] = s
	m.mu.Unlock()
}

// Get returns the session for a code, or nil.
func (m *Manager) Get(code string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[code]
}

// Remove closes and forgets a session.
func (m *Manager) Remove(code string) {
	m.mu.Lock()
	s := m.sessions[code]
	delete(m.sessions, code)
	m.mu.Unlock()
	if s != nil {
		s.Close()
	}
}

// Count is the number of live sessions (metrics).
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// Each calls fn for every session.
func (m *Manager) Each(fn func(*Session)) {
	m.mu.RLock()
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.mu.RUnlock()
	for _, s := range list {
		fn(s)
	}
}
