package hub

import (
	"embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/stranix79/deckhand/internal/deck"
)

//go:embed templates/*.html
var templateFS embed.FS

// parseTemplates loads layout + one template set per page. Each page file
// defines "content"; we clone the layout for each so names do not clash.
func parseTemplates() (*template.Template, error) {
	return template.ParseFS(templateFS, "templates/layout.html")
}

func (h *Hub) page(name string) (*template.Template, error) {
	t, err := h.tmpl.Clone()
	if err != nil {
		return nil, err
	}
	return t.ParseFS(templateFS, "templates/"+name)
}

// pageCSP allows the landing's Google Fonts and inline styles; scripts stay
// same-origin (plus inline for the landing's language switch).
const pageCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; frame-src 'self'; connect-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'self'"

func (h *Hub) render(w http.ResponseWriter, r *http.Request, name string, data map[string]any) {
	t, err := h.page(name)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["User"]; !ok {
		if u := userOf(r); u != nil {
			data["User"] = u
		} else {
			data["User"] = h.userFromRequest(r)
		}
	}
	data["Cfg"] = h.cfg
	data["BaseURL"] = h.cfg.BaseURL
	data["Year"] = time.Now().Year()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", pageCSP)
	w.Header().Set("Cache-Control", "no-store")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("render", "page", name, "err", err)
	}
}

func (h *Hub) serverError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "path", r.URL.Path, "err", err)
	w.WriteHeader(http.StatusInternalServerError)
	h.render(w, r, "error.html", map[string]any{"Title": "Something went wrong", "Message": "The hub hit an error. It has been logged; please try again."})
}

func (h *Hub) notFound(w http.ResponseWriter, r *http.Request, msg string) {
	w.WriteHeader(http.StatusNotFound)
	h.render(w, r, "error.html", map[string]any{"Title": "Not found", "Message": msg})
}

// --- /app ------------------------------------------------------------------------------

type liveRow struct {
	ID, Title, Mode, Code, Token string
	StartedAt                    time.Time
}

func (h *Hub) appData(r *http.Request, extra map[string]any) (map[string]any, error) {
	u := userOf(r)
	decks, err := h.listDecks(r.Context(), u.ID)
	if err != nil {
		return nil, err
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT p.id, d.title, p.mode, p.code, p.token, p.started_at, p.ended_at
		FROM presentations p JOIN decks d ON d.id = p.deck_id
		WHERE p.user_id = $1 AND p.mode <> 'permalink' ORDER BY p.started_at DESC LIMIT 30`, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var live, past []liveRow
	for rows.Next() {
		var l liveRow
		var ended *time.Time
		if err := rows.Scan(&l.ID, &l.Title, &l.Mode, &l.Code, &l.Token, &l.StartedAt, &ended); err != nil {
			return nil, err
		}
		if ended == nil {
			live = append(live, l)
		} else if len(past) < 10 {
			past = append(past, l)
		}
	}
	data := map[string]any{"Title": "Decks", "Decks": decks, "Live": live, "Past": past}
	for k, v := range extra {
		data[k] = v
	}
	return data, nil
}

func (h *Hub) appPage(w http.ResponseWriter, r *http.Request) {
	data, err := h.appData(r, nil)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.render(w, r, "app.html", data)
}

func (h *Hub) uploadDeckForm(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxDeckBytes+1<<20)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		h.appError(w, r, "Choose a .zip or .tar.gz file.", nil)
		return
	}
	defer func() { _ = f.Close() }()
	_, err = h.storeDeck(r.Context(), u, f, hdr.Filename, r.FormValue("slug"))
	if err != nil {
		var rep *deck.Report
		switch {
		case errors.As(err, &rep):
			h.appError(w, r, "The deck is not valid:", rep.Errors)
		case errors.Is(err, ErrPlanLimit):
			h.appError(w, r, "The free plan allows one deck. Delete it, upload with the same slug to replace it, or upgrade.", nil)
		default:
			h.serverError(w, r, err)
		}
		return
	}
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

func (h *Hub) appError(w http.ResponseWriter, r *http.Request, msg string, list []string) {
	data, err := h.appData(r, map[string]any{"Error": msg, "Errors": list})
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusUnprocessableEntity)
	h.render(w, r, "app.html", data)
}

func (h *Hub) presentNow(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	d, err := h.getDeck(r.Context(), chi.URLParam(r, "id"))
	if err != nil || d.UserID != u.ID {
		h.notFound(w, r, "No such deck.")
		return
	}
	p, s, err := h.startPresentation(r.Context(), d, u, "hosted")
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.render(w, r, "started.html", map[string]any{
		"Title": "Live", "Deck": d, "Code": p.Code,
		"StageURL":  h.cfg.BaseURL + "/s/" + p.Code + "?t=" + p.Token,
		"RemoteURL": h.cfg.BaseURL + "/r/" + p.Code + "?t=" + p.Token,
		"ViewerURL": s.ViewerURL,
	})
}

func (h *Hub) deleteDeckForm(w http.ResponseWriter, r *http.Request) {
	if err := h.deleteDeck(r.Context(), userOf(r), chi.URLParam(r, "id")); err != nil {
		h.notFound(w, r, "No such deck.")
		return
	}
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

func (h *Hub) endPresentationForm(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	p, err := h.getPresentation(r.Context(), chi.URLParam(r, "id"))
	if err != nil || p.UserID != u.ID {
		h.notFound(w, r, "No such presentation.")
		return
	}
	_ = h.endPresentation(r.Context(), p.ID)
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

// --- /d/{handle}/{slug} --------------------------------------------------------------------

func (h *Hub) deckPermalink(w http.ResponseWriter, r *http.Request) {
	handle, slug := strings.ToLower(chi.URLParam(r, "handle")), strings.ToLower(chi.URLParam(r, "slug"))
	d, err := h.getDeckByHandleSlug(r.Context(), handle, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.notFound(w, r, "No deck at this address.")
			return
		}
		h.serverError(w, r, err)
		return
	}
	if d.Expired() {
		w.WriteHeader(http.StatusGone)
		h.render(w, r, "gone.html", map[string]any{"Title": "Link expired"})
		return
	}
	code, err := h.permalinkSession(r.Context(), d)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	t, err := h.page("deck.html")
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", pageCSP)
	w.Header().Set("Cache-Control", "public, max-age=300")
	data := map[string]any{
		"Title": d.Title, "Deck": d, "Handle": handle, "Code": code, "User": h.userFromRequest(r),
		"Cfg": h.cfg, "BaseURL": h.cfg.BaseURL,
		"OG": map[string]string{"title": d.Title, "url": h.deckURL(handle, slug), "description": "A Deckhand presentation by " + handle},
	}
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("render permalink", "err", err)
	}
}
