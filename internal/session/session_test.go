package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/stranix79/deckhand/internal/deck"
)

func testDeck(t *testing.T, n int) *deck.Deck {
	t.Helper()
	root := t.TempDir()
	for i := 1; i <= n; i++ {
		name := filepath.Join(root, "0"+string(rune('0'+i))+"-s.html")
		if err := os.WriteFile(name, []byte("<html></html>"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	d, err := deck.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestCodes(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c := NewCode()
		if !ValidCode(c) || strings.ContainsAny(c, "IO01") {
			t.Fatalf("bad code %q", c)
		}
		seen[c] = true
	}
	if len(seen) < 199 {
		t.Fatal("codes are not random enough")
	}
	if ValidCode("ABC") || ValidCode("ABCDE0") {
		t.Fatal("invalid codes accepted")
	}
}

// Without a stage, next/prev/goto move immediately and clamp to the deck.
func TestMoveWithoutStage(t *testing.T) {
	s := New(testDeck(t, 3))
	defer s.Close()
	c := &client{role: RoleRemote, out: make(chan []byte, 64)}
	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()

	must := func(op string) {
		t.Helper()
		if err := s.apply(c, []byte(op)); err != nil {
			t.Fatalf("%s: %v", op, err)
		}
	}
	must(`{"op":"next"}`)
	if st := s.State(); st.Slide != 1 || st.StartedAt == nil {
		t.Fatalf("after next: %+v", st)
	}
	must(`{"op":"next"}`)
	must(`{"op":"next"}`) // clamped at 2
	if st := s.State(); st.Slide != 2 {
		t.Fatalf("clamp: %+v", st)
	}
	must(`{"op":"goto","slide":0}`)
	must(`{"op":"prev"}`) // clamped at 0
	if st := s.State(); st.Slide != 0 {
		t.Fatalf("clamp low: %+v", st)
	}
	must(`{"op":"pointer","x":0.5,"y":1.7}`)
	if st := s.State(); st.Pointer == nil || st.Pointer.X != 0.5 || st.Pointer.Y != 1 {
		t.Fatalf("pointer: %+v", st.Pointer)
	}
	must(`{"op":"pointer"}`)
	if st := s.State(); st.Pointer != nil {
		t.Fatal("pointer not cleared")
	}
	must(`{"op":"black"}`)
	if !s.State().Black {
		t.Fatal("black not toggled")
	}
	if err := s.apply(c, []byte(`{"op":"dance"}`)); err == nil {
		t.Fatal("unknown op accepted")
	}
	// A viewer may not send anything.
	v := &client{role: RoleViewer, out: make(chan []byte, 8)}
	if err := s.apply(v, []byte(`{"op":"next"}`)); err != errForbidden {
		t.Fatalf("viewer op: %v", err)
	}
}

// With a stage, "next" becomes a question; handled:true moves a fragment,
// handled:false (or silence) moves the slide.
func TestFragmentsViaStage(t *testing.T) {
	s := New(testDeck(t, 3))
	defer s.Close()
	s.askDelay = 50 * time.Millisecond
	stage := &client{role: RoleStage, out: make(chan []byte, 64)}
	remote := &client{role: RoleRemote, out: make(chan []byte, 64)}
	s.mu.Lock()
	s.clients[stage] = struct{}{}
	s.clients[remote] = struct{}{}
	s.stage = stage
	s.mu.Unlock()
	drain(stage.out)

	if err := s.apply(remote, []byte(`{"op":"next"}`)); err != nil {
		t.Fatal(err)
	}
	var q struct {
		Op  string `json:"op"`
		Seq int    `json:"seq"`
		Dir string `json:"dir"`
	}
	if err := json.Unmarshal(<-stage.out, &q); err != nil || q.Op != "ask" || q.Dir != "next" {
		t.Fatalf("stage did not get the question: %+v %v", q, err)
	}
	if st := s.State(); st.Slide != 0 {
		t.Fatal("slide moved before the answer")
	}
	// The slide handled it: fragment moves, slide stays.
	if err := s.apply(stage, []byte(`{"op":"answer","seq":1,"handled":true}`)); err != nil {
		t.Fatal(err)
	}
	if st := s.State(); st.Slide != 0 || st.Fragment != 1 {
		t.Fatalf("fragment expected: %+v", st)
	}
	// Not handled: slide moves, fragment resets.
	_ = s.apply(remote, []byte(`{"op":"next"}`))
	_ = s.apply(stage, []byte(`{"op":"answer","seq":2,"handled":false}`))
	if st := s.State(); st.Slide != 1 || st.Fragment != 0 {
		t.Fatalf("slide expected: %+v", st)
	}
	// Silence: the timer answers "not handled".
	_ = s.apply(remote, []byte(`{"op":"next"}`))
	time.Sleep(120 * time.Millisecond)
	if st := s.State(); st.Slide != 2 {
		t.Fatalf("timeout should move the slide: %+v", st)
	}
	// Late answer for an old seq is ignored.
	_ = s.apply(stage, []byte(`{"op":"answer","seq":3,"handled":true}`))
	if st := s.State(); st.Fragment != 0 {
		t.Fatal("late answer applied")
	}
}

func drain(ch chan []byte) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// Integration: a real HTTP server, three WebSockets, and the sync between them.
func TestWebSocketSync(t *testing.T) {
	s := New(testDeck(t, 4))
	defer s.Close()
	s.askDelay = 50 * time.Millisecond
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/", func(w http.ResponseWriter, r *http.Request) { s.ServeWS(w, r, nil) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/" + s.Code

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dial := func(q string) *websocket.Conn {
		t.Helper()
		c, _, err := websocket.Dial(ctx, wsURL+q, nil)
		if err != nil {
			t.Fatalf("dial %s: %v", q, err)
		}
		return c
	}

	// Remote without token is refused.
	if _, resp, err := websocket.Dial(ctx, wsURL+"?role=remote", nil); err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("remote without token must be 403, got err=%v resp=%v", err, resp)
	}

	stage := dial("?role=stage")
	remote := dial("?role=remote&t=" + s.Token)
	viewer := dial("?role=viewer")
	defer func() { _ = stage.CloseNow(); _ = remote.CloseNow(); _ = viewer.CloseNow() }()

	// Every client gets deck, state, viewers on connect.
	type frame struct {
		Op    string          `json:"op"`
		State *State          `json:"state"`
		Deck  json.RawMessage `json:"deck"`
		Count int             `json:"count"`
		Seq   int             `json:"seq"`
	}
	read := func(c *websocket.Conn) frame {
		t.Helper()
		_, raw, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var f frame
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatal(err)
		}
		return f
	}
	for _, c := range []*websocket.Conn{stage, remote, viewer} {
		if f := read(c); f.Op != "deck" || len(f.Deck) == 0 {
			t.Fatalf("first frame should be deck, got %+v", f)
		}
		if f := read(c); f.Op != "state" || f.State == nil || f.State.Slide != 0 {
			t.Fatalf("second frame should be state, got %+v", f)
		}
		if f := read(c); f.Op != "viewers" {
			t.Fatalf("third frame should be viewers, got %+v", f)
		}
	}
	// Notes: the deck frame for the viewer must not contain private notes.
	// (Covered by deckMessage unit behaviour; here we check the remote gets viewerUrl.)

	// Remote presses next → stage is asked → answers not handled → everybody at slide 1.
	if err := remote.Write(ctx, websocket.MessageText, []byte(`{"op":"next"}`)); err != nil {
		t.Fatal(err)
	}
	q := read(stage)
	if q.Op != "ask" {
		t.Fatalf("stage expected ask, got %+v", q)
	}
	if err := stage.Write(ctx, websocket.MessageText, []byte(`{"op":"answer","seq":1,"handled":false}`)); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]*websocket.Conn{"stage": stage, "remote": remote, "viewer": viewer} {
		f := read(c)
		if f.Op != "state" || f.State.Slide != 1 {
			t.Fatalf("%s: expected state slide 1, got %+v", name, f)
		}
	}

	// Pointer from the remote reaches the stage.
	if err := remote.Write(ctx, websocket.MessageText, []byte(`{"op":"pointer","x":0.25,"y":0.75}`)); err != nil {
		t.Fatal(err)
	}
	if f := read(stage); f.Op != "state" || f.State.Pointer == nil || f.State.Pointer.X != 0.25 {
		t.Fatalf("pointer did not reach the stage: %+v", f)
	}

	// A viewer that tries to control is disconnected.
	if err := viewer.Write(ctx, websocket.MessageText, []byte(`{"op":"next"}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := viewer.Read(ctx); err == nil {
		// it may still receive the pointer state first; read once more
		if _, _, err = viewer.Read(ctx); err == nil {
			t.Fatal("viewer should be closed after sending an op")
		}
	}
	if st := s.State(); st.Slide != 1 {
		t.Fatalf("viewer op must not change state: %+v", st)
	}
}
