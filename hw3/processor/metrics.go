package main

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var processed = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "image_processing_total", Help: "Processing attempts, including Kafka redeliveries.",
}, []string{"filter", "status"})
var processingTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name: "image_processing_duration_seconds", Help: "Decode, filter and PNG write duration; excludes Kafka and callback wait.",
	Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 2, 5, 10, 30},
}, []string{"filter"})

func init() { prometheus.MustRegister(processed, processingTime) }

func startMetrics() (func(), error) {
	listener, err := net.Listen("tcp", ":9091")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	return func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		server.Shutdown(stopCtx)
	}, nil
}

func recordProcessing(name string, started time.Time, err error) {
	switch name {
	case "negative", "flip_x", "blur", "sharpen":
	default:
		name = "unknown"
	}
	status := "success"
	if err != nil {
		status = "failed"
	}
	processed.WithLabelValues(name, status).Inc()
	processingTime.WithLabelValues(name).Observe(time.Since(started).Seconds())
}
