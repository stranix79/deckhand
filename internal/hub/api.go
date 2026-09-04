package hub

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/stranix79/deckhand/internal/deck"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// apiPushDeck: multipart form, field "file" (.zip/.tar.gz), optional "slug".
func (h *Hub) apiPushDeck(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxDeckBytes+1<<20)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "multipart field \"file\" is required"})
		return
	}
	defer func() { _ = f.Close() }()
	d, err := h.storeDeck(r.Context(), u, f, hdr.Filename, r.FormValue("slug"))
	if err != nil {
		var rep *deck.Report
		switch {
		case errors.As(err, &rep):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "invalid deck", "errors": rep.Errors})
		case errors.Is(err, ErrPlanLimit):
			writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "free plan: 1 deck. Delete a deck, reuse its slug (--slug), or subscribe at " + h.cfg.BaseURL + "/billing"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": d.ID, "slug": d.Slug, "title": d.Title, "version": d.Version, "slides": d.SlideCount,
		"url": h.deckURL(u.Handle, d.Slug), "expires_at": d.ExpiresAt,
	})
}

// apiStartPresentation: JSON {"deck_id": …} or {"slug": …}, mode relay.
func (h *Hub) apiStartPresentation(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	var in struct {
		DeckID string `json:"deck_id"`
		Slug   string `json:"slug"`
		Mode   string `json:"mode"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	var d *DeckRow
	var err error
	switch {
	case in.DeckID != "":
		d, err = h.getDeck(r.Context(), in.DeckID)
	case in.Slug != "":
		d, err = h.getDeckBySlug(r.Context(), u.ID, in.Slug)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "deck_id or slug is required"})
		return
	}
	if err != nil || d.UserID != u.ID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "deck not found"})
		return
	}
	mode := "relay"
	if in.Mode == "hosted" {
		mode = "hosted"
	}
	p, s, err := h.startPresentation(r.Context(), d, u, mode)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	ws := "wss://"
	if !h.cfg.Secure() {
		ws = "ws://"
	}
	host := strings.TrimPrefix(strings.TrimPrefix(h.cfg.BaseURL, "https://"), "http://")
	writeJSON(w, http.StatusOK, map[string]any{
		"id": p.ID, "code": p.Code, "token": p.Token,
		"viewer_url": s.ViewerURL,
		"stage_url":  h.cfg.BaseURL + "/s/" + p.Code + "?t=" + p.Token,
		"remote_url": h.cfg.BaseURL + "/r/" + p.Code + "?t=" + p.Token,
		"relay_url":  ws + host + "/api/v1/relay/" + p.Code + "?t=" + p.Token,
		"stats_url":  h.cfg.BaseURL + "/app/presentations/" + p.ID + "/stats",
	})
}

var _ = pgx.ErrNoRows
