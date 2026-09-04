package local

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/stranix79/deckhand/internal/session"
)

// HubClient talks to a Deckhand hub with an API token (`deckhand login`).
type HubClient struct {
	URL   string // https://deckhand.example.com
	Token string
	HTTP  *http.Client
}

// PushResult is the hub's answer to a deck upload.
type PushResult struct {
	ID        string     `json:"id"`
	Slug      string     `json:"slug"`
	Title     string     `json:"title"`
	Version   int        `json:"version"`
	Slides    int        `json:"slides"`
	URL       string     `json:"url"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// StartResult is the hub's answer to "start a relayed presentation".
type StartResult struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Token     string `json:"token"`
	ViewerURL string `json:"viewer_url"`
	StageURL  string `json:"stage_url"`
	RemoteURL string `json:"remote_url"`
	RelayURL  string `json:"relay_url"`
	StatsURL  string `json:"stats_url"`
}

func (c *HubClient) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func (c *HubClient) do(ctx context.Context, method, path string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.URL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error  string   `json:"error"`
			Errors []string `json:"errors"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error == "" {
			e.Error = strings.TrimSpace(string(raw))
		}
		if len(e.Errors) > 0 {
			e.Error += ": " + strings.Join(e.Errors, "; ")
		}
		return fmt.Errorf("hub %s %s: %s (HTTP %d)", method, path, e.Error, resp.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// Me checks the token.
func (c *HubClient) Me(ctx context.Context) (email, handle, plan string, err error) {
	var out struct{ Email, Handle, Plan string }
	err = c.do(ctx, http.MethodGet, "/api/v1/me", nil, "", &out)
	return out.Email, out.Handle, out.Plan, err
}

// Push uploads a deck (directory → zipped in memory; archive → as-is).
func (c *HubClient) Push(ctx context.Context, path, slug string) (*PushResult, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if slug != "" {
		_ = mw.WriteField("slug", slug)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	if info.IsDir() {
		name = strings.TrimSuffix(name, "/") + ".zip"
	}
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		if err := zipDir(path, fw); err != nil {
			return nil, err
		}
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(fw, f)
		_ = f.Close()
		if err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	var out PushResult
	if err := c.do(ctx, http.MethodPost, "/api/v1/decks", &body, mw.FormDataContentType(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Start creates a relayed presentation of a pushed deck.
func (c *HubClient) Start(ctx context.Context, deckID string) (*StartResult, error) {
	raw, _ := json.Marshal(map[string]string{"deck_id": deckID, "mode": "relay"})
	var out StartResult
	if err := c.do(ctx, http.MethodPost, "/api/v1/presentations", bytes.NewReader(raw), "application/json", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Relay keeps a WebSocket to the hub and forwards every state from states.
// It reconnects with backoff; a hub outage never affects the local session.
// onViewers is called with the remote viewer count; onStatus with a short
// message for the terminal.
func Relay(ctx context.Context, relayURL string, states <-chan session.State, onViewers func(int), onStatus func(string)) {
	delay := time.Second
	var last *session.State
	for ctx.Err() == nil {
		err := relayOnce(ctx, relayURL, states, &last, onViewers, onStatus)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			onStatus(fmt.Sprintf("hub connection lost (%v), retrying in %s", err, delay))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}
}

func relayOnce(ctx context.Context, relayURL string, states <-chan session.State, last **session.State, onViewers func(int), onStatus func(string)) error {
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	conn, _, err := websocket.Dial(dctx, relayURL, nil)
	cancel()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "bye") }()
	onStatus("hub connected")
	send := func(v any) error {
		raw, _ := json.Marshal(v)
		wctx, wcancel := context.WithTimeout(ctx, 10*time.Second)
		defer wcancel()
		return conn.Write(wctx, websocket.MessageText, raw)
	}
	// Replay the last known state so the hub catches up after an outage.
	if *last != nil {
		if err := send(map[string]any{"op": "state", "state": *last, "ts": time.Now().UnixMilli()}); err != nil {
			return err
		}
	}
	errc := make(chan error, 1)
	go func() {
		for {
			_, raw, err := conn.Read(ctx)
			if err != nil {
				errc <- err
				return
			}
			var f struct {
				Op    string `json:"op"`
				Count int    `json:"count"`
			}
			if json.Unmarshal(raw, &f) == nil && f.Op == "viewers" {
				onViewers(f.Count)
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			_ = send(map[string]any{"op": "end"})
			return nil
		case err := <-errc:
			return err
		case st := <-states:
			cp := st
			*last = &cp
			if err := send(map[string]any{"op": "state", "state": st, "ts": time.Now().UnixMilli()}); err != nil {
				return err
			}
		}
	}
}

// zipDir zips a deck directory (skipping OS litter) into w. It is the
// user's own directory, already validated by deck.Load.
func zipDir(dir string, w io.Writer) error { //nolint:gosec // see above
	zw := zip.NewWriter(w)
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." {
			return nil
		}
		base := d.Name()
		if base == ".DS_Store" || base == "__MACOSX" || base == "Thumbs.db" || strings.HasPrefix(base, "._") || base == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return errors.New(rel + ": only regular files can be pushed")
		}
		f, err := os.Open(p) //nolint:gosec // user's own deck
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		fw, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		_, err = io.Copy(fw, f)
		return err
	})
	if err != nil {
		return err
	}
	return zw.Close()
}

var _ = slog.Info
