package hub

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/stranix79/deckhand/internal/session"
)

// metrics exposed on /metrics (brief §7): open WebSockets, active sessions,
// viewers, relay latency, plus HTTP requests.
type metrics struct {
	reg          *prometheus.Registry
	requests     *prometheus.CounterVec
	relays       prometheus.Gauge
	relayLatency prometheus.Histogram
}

func newMetrics() *metrics {
	m := &metrics{reg: prometheus.NewRegistry()}
	m.requests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "deckhand_http_requests_total", Help: "HTTP requests by status class."}, []string{"status"})
	m.relays = prometheus.NewGauge(prometheus.GaugeOpts{Name: "deckhand_relay_connections", Help: "Open relay WebSockets from local presentations."})
	m.relayLatency = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "deckhand_relay_apply_seconds", Help: "Time to apply and broadcast a relayed state.", Buckets: prometheus.ExponentialBuckets(0.0001, 4, 8)})
	m.reg.MustRegister(m.requests, m.relays, m.relayLatency)
	return m
}

// bind registers the gauges that read live values from the session manager.
func (m *metrics) bind(mgr *session.Manager) {
	m.reg.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "deckhand_sessions_active", Help: "Sessions held in memory."}, func() float64 { return float64(mgr.Count()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "deckhand_ws_connections", Help: "Open WebSockets (all roles)."}, func() float64 {
			n := 0
			mgr.Each(func(s *session.Session) { n += s.Clients() })
			return float64(n)
		}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "deckhand_viewers", Help: "Connected viewers."}, func() float64 {
			n := 0
			mgr.Each(func(s *session.Session) { n += s.Viewers() })
			return float64(n)
		}),
	)
}

func (m *metrics) handler() http.Handler { return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{}) }

func (m *metrics) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		_ = start
		m.requests.WithLabelValues(strconv.Itoa(ww.Status()/100) + "xx").Inc()
	})
}
