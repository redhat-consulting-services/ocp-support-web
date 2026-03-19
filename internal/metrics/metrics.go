package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ocp_support_web_http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ocp_support_web_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	MustGatherJobsActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ocp_support_web_mustgather_jobs_active",
			Help: "Number of currently running must-gather jobs.",
		},
	)

	MustGatherJobsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ocp_support_web_mustgather_jobs_total",
			Help: "Total number of must-gather jobs started.",
		},
		[]string{"type"},
	)

	EtcdDiagJobsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "ocp_support_web_etcd_diag_jobs_total",
			Help: "Total number of etcd diagnostic jobs started.",
		},
	)
)

func init() {
	prometheus.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestDuration,
		MustGatherJobsActive,
		MustGatherJobsTotal,
		EtcdDiagJobsTotal,
	)
}

func Handler() http.Handler {
	return promhttp.Handler()
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		if len(r.URL.Path) > 8 && r.URL.Path[:8] == "/static/" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		duration := time.Since(start).Seconds()

		path := normalizePath(r.URL.Path)
		HTTPRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(sw.status)).Inc()
		HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}

func normalizePath(path string) string {
	switch {
	case path == "/", path == "/status":
		return path
	case len(path) > 25 && path[:25] == "/api/support/gather/" && path[len(path)-9:] == "/download":
		return "/api/support/gather/{id}/download"
	case len(path) > 20 && path[:20] == "/api/support/gather/":
		return "/api/support/gather/{id}"
	case len(path) > 23 && path[:23] == "/api/support/etcd-diag/":
		return "/api/support/etcd-diag/{id}"
	default:
		return path
	}
}
