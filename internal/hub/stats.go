package hub

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Stats of one presentation, computed from viewers_events.
type Stats struct {
	Unique     int
	Peak       int
	Total      time.Duration
	TotalHuman string
	Slides     []SlideStat
}

// SlideStat is one visited slide (in order of first visit).
type SlideStat struct {
	Index         int // 1-based for display
	Duration      time.Duration
	DurationHuman string
	Viewers       int // audience present when the slide was shown (max over visits)
}

type evt struct {
	at     time.Time
	kind   string
	viewer string
	slide  int
}

// computeStats replays the events in time order: joins/leaves give the
// audience size at any moment, slide events give the timeline.
func (h *Hub) computeStats(ctx context.Context, p *PresRow) (*Stats, error) {
	rows, err := h.db.Query(ctx, `SELECT at, kind, viewer_id, slide FROM viewers_events WHERE presentation_id = $1 ORDER BY at, id`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []evt
	for rows.Next() {
		var e evt
		if err := rows.Scan(&e.at, &e.kind, &e.viewer, &e.slide); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return replay(events, p.StartedAt, p.EndedAt), nil
}

func replay(events []evt, startedAt time.Time, endedAt *time.Time) *Stats {
	st := &Stats{}
	end := time.Now()
	if endedAt != nil {
		end = *endedAt
	}
	seen := map[string]bool{}
	present := 0
	cur := 0
	curSince := startedAt
	bySlide := map[int]*SlideStat{}
	order := []int{}
	touch := func(i int) *SlideStat {
		s, ok := bySlide[i]
		if !ok {
			s = &SlideStat{Index: i + 1}
			bySlide[i] = s
			order = append(order, i)
		}
		return s
	}
	touch(0)
	for _, e := range events {
		switch e.kind {
		case "join":
			if !seen[e.viewer] {
				seen[e.viewer] = true
				st.Unique++
			}
			present++
			if present > st.Peak {
				st.Peak = present
			}
			if s := touch(cur); present > s.Viewers {
				s.Viewers = present
			}
		case "leave":
			if present > 0 {
				present--
			}
		case "slide":
			touch(cur).Duration += e.at.Sub(curSince)
			cur, curSince = e.slide, e.at
			if s := touch(cur); present > s.Viewers {
				s.Viewers = present
			}
		}
	}
	touch(cur).Duration += end.Sub(curSince)
	sort.Ints(order)
	for _, i := range order {
		s := bySlide[i]
		s.DurationHuman = humanDuration(s.Duration)
		st.Total += s.Duration
		st.Slides = append(st.Slides, *s)
	}
	st.TotalHuman = humanDuration(st.Total)
	return st
}

func humanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// statsSVG draws the audience-per-slide bars, server side, no JS library.
func statsSVG(st *Stats) string {
	const w, hgt, padL, padB, padT = 800, 260, 36, 28, 16
	n := len(st.Slides)
	if n == 0 {
		return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 80"><text x="400" y="45" text-anchor="middle" font-family="sans-serif" fill="#5C6B7A">No audience data</text></svg>`
	}
	maxV := st.Peak
	if maxV < 1 {
		maxV = 1
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" font-family="Bricolage Grotesque, Helvetica Neue, Arial, sans-serif" font-size="12">`, w, hgt)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#ffffff"/>`, w, hgt)
	plotW, plotH := float64(w-padL-8), float64(hgt-padT-padB)
	// gridlines
	for _, f := range []float64{0, .5, 1} {
		y := float64(padT) + plotH - f*plotH
		fmt.Fprintf(&b, `<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="#E9EEEF"/>`, padL, y, w-8, y)
		fmt.Fprintf(&b, `<text x="%d" y="%.1f" text-anchor="end" fill="#5C6B7A" dy="4">%d</text>`, padL-6, y, int(float64(maxV)*f+.5))
	}
	bw := plotW / float64(n)
	for i, s := range st.Slides {
		bh := plotH * float64(s.Viewers) / float64(maxV)
		x := float64(padL) + float64(i)*bw
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#1E6E74" rx="2"><title>slide %d: %d viewers, %s</title></rect>`,
			x+bw*0.15, float64(padT)+plotH-bh, bw*0.7, bh, s.Index, s.Viewers, s.DurationHuman)
		if n <= 40 || i%5 == 0 {
			fmt.Fprintf(&b, `<text x="%.1f" y="%d" text-anchor="middle" fill="#12233B">%d</text>`, x+bw/2, hgt-8, s.Index)
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func (h *Hub) statsPage(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	p, err := h.getPresentation(r.Context(), chi.URLParam(r, "id"))
	if err != nil || p.UserID != u.ID {
		h.notFound(w, r, "No such presentation.")
		return
	}
	d, err := h.getDeck(r.Context(), p.DeckID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	st, err := h.computeStats(r.Context(), p)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.render(w, r, "stats.html", map[string]any{"Title": "Statistics", "Deck": d, "Pres": p, "Stats": st})
}

func (h *Hub) statsSVG(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	p, err := h.getPresentation(r.Context(), chi.URLParam(r, "id"))
	if err != nil || p.UserID != u.ID {
		http.NotFound(w, r)
		return
	}
	st, err := h.computeStats(r.Context(), p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(statsSVG(st)))
}
