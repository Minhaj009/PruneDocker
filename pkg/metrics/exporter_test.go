package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsRegistrationAndScrape(t *testing.T) {
	m := NewMetrics()

	// Update values
	m.ReclaimedBytesTotal.Add(1024 * 1024 * 50) // 50 MB
	m.PrunedImagesTotal.Add(3)
	m.PrunedVolumesTotal.Add(1)
	m.WarmCacheBytes.Set(1024 * 1024 * 500) // 500 MB
	m.DiskUsageRatio.Set(0.75)
	m.RunsTotal.WithLabelValues("success", "live").Inc()

	server := NewMetricsServer(":0", m)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	server.Handler.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK from /metrics, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	bodyStr := string(body)

	expectedMetrics := []string{
		"prunedocker_reclaimed_bytes_total",
		"prunedocker_pruned_images_total",
		"prunedocker_pruned_volumes_total",
		"prunedocker_warm_cache_bytes",
		"prunedocker_disk_usage_ratio",
		"prunedocker_runs_total",
	}

	for _, metricName := range expectedMetrics {
		if !strings.Contains(bodyStr, metricName) {
			t.Errorf("expected /metrics output to contain %q", metricName)
		}
	}
}

func TestHealthzEndpoint(t *testing.T) {
	m := NewMetrics()
	server := NewMetricsServer(":0", m)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	server.Handler.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK from /healthz, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if strings.TrimSpace(string(body)) != "OK" {
		t.Errorf("expected 'OK', got %q", string(body))
	}
}

func TestShutdownServer(t *testing.T) {
	err := ShutdownServer(context.Background(), nil)
	if err != nil {
		t.Errorf("expected nil error on nil server shutdown, got %v", err)
	}
}
