package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "image_api_requests_total", Help: "HTTP requests by method, route pattern and status.",
}, []string{"method", "route", "status"})
var httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name: "image_api_request_duration_seconds", Help: "HTTP request duration.",
	Buckets: prometheus.DefBuckets,
}, []string{"method", "route"})

func init() { prometheus.MustRegister(httpRequests, httpDuration) }

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func observeHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" || r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		started := time.Now()
		wrapped := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(wrapped, r)
		if wrapped.status == 0 {
			wrapped.status = http.StatusOK
		}
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		method := r.Method
		switch method {
		case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
		default:
			method = "OTHER"
		}
		// Метки содержат шаблон маршрута, а не UUID/логин/токен.
		httpRequests.WithLabelValues(method, route, strconv.Itoa(wrapped.status)).Inc()
		httpDuration.WithLabelValues(method, route).Observe(time.Since(started).Seconds())
	})
}
