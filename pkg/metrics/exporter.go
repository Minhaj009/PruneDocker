package metrics

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the registered Prometheus collectors for PruneDocker.
type Metrics struct {
	Registry            *prometheus.Registry
	ReclaimedBytesTotal prometheus.Counter
	PrunedImagesTotal   prometheus.Counter
	PrunedVolumesTotal  prometheus.Counter
	WarmCacheBytes      prometheus.Gauge
	DiskUsageRatio      prometheus.Gauge
	RunsTotal           *prometheus.CounterVec
}

// NewMetrics initializes and registers all PruneDocker Prometheus metrics.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	m := &Metrics{
		Registry: reg,
		ReclaimedBytesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "prunedocker",
			Name:      "reclaimed_bytes_total",
			Help:      "Total bytes of disk space successfully reclaimed by pruning operations.",
		}),
		PrunedImagesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "prunedocker",
			Name:      "pruned_images_total",
			Help:      "Total count of intermediate leaf images pruned.",
		}),
		PrunedVolumesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "prunedocker",
			Name:      "pruned_volumes_total",
			Help:      "Total count of orphaned anonymous volumes pruned.",
		}),
		WarmCacheBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "prunedocker",
			Name:      "warm_cache_bytes",
			Help:      "Total bytes of warm, high-reuse build cache currently protected.",
		}),
		DiskUsageRatio: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "prunedocker",
			Name:      "disk_usage_ratio",
			Help:      "Current Docker storage disk usage ratio (0.0 to 1.0).",
		}),
		RunsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "prunedocker",
				Name:      "runs_total",
				Help:      "Total number of optimization runs executed.",
			},
			[]string{"status", "mode"},
		),
	}

	reg.MustRegister(
		m.ReclaimedBytesTotal,
		m.PrunedImagesTotal,
		m.PrunedVolumesTotal,
		m.WarmCacheBytes,
		m.DiskUsageRatio,
		m.RunsTotal,
	)

	return m
}

// NewMetricsServer creates an http.Server configured with the /metrics and /healthz endpoints.
func NewMetricsServer(addr string, m *Metrics) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK\n"))
	})

	return &http.Server{
		Addr:    addr,
		Handler: mux,
	}
}

// ShutdownServer cleanly shuts down the HTTP server.
func ShutdownServer(ctx context.Context, server *http.Server) error {
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}
