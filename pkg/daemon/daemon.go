package daemon

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/prunedocker/prunedocker/pkg/analyzer"
	"github.com/prunedocker/prunedocker/pkg/cleaner"
	"github.com/prunedocker/prunedocker/pkg/docker"
	"github.com/prunedocker/prunedocker/pkg/metrics"
)

// Config configures the background daemon process.
type Config struct {
	MaxDiskUsagePercent float64                `json:"max_disk_usage_percent"` // e.g. 80.0
	CheckInterval       time.Duration          `json:"check_interval"`          // e.g. 30m
	MetricsAddr         string                 `json:"metrics_addr"`           // e.g. ":9199"
	DryRun              bool                   `json:"dry_run"`
	Policy              analyzer.ScoringPolicy `json:"policy"`
}

// DefaultConfig provides recommended defaults for daemon mode.
func DefaultConfig() Config {
	return Config{
		MaxDiskUsagePercent: 80.0,
		CheckInterval:       30 * time.Minute,
		MetricsAddr:         ":9199",
		DryRun:              false,
		Policy:              analyzer.DefaultPolicy(),
	}
}

// Daemon manages background recurring evaluation, watermark checks, and metrics emission.
type Daemon struct {
	client  docker.DockerClient
	config  Config
	metrics *metrics.Metrics
	pruner  *cleaner.Pruner
}

// NewDaemon creates a new Daemon service instance.
func NewDaemon(client docker.DockerClient, config Config) *Daemon {
	m := metrics.NewMetrics()
	return &Daemon{
		client:  client,
		config:  config,
		metrics: m,
		pruner:  cleaner.NewPruner(client),
	}
}

// GetMetrics returns the internal Prometheus metrics instance.
func (d *Daemon) GetMetrics() *metrics.Metrics {
	return d.metrics
}

// RunOnce performs a single inspection, DAG analysis, and conditional pruning run.
func (d *Daemon) RunOnce(ctx context.Context) (*cleaner.PruneReport, error) {
	// 1. Inspect Docker state
	snapshot, err := docker.InspectEngine(ctx, d.client)
	if err != nil {
		modeStr := "live"
		if d.config.DryRun {
			modeStr = "dry_run"
		}
		d.metrics.RunsTotal.WithLabelValues("error", modeStr).Inc()
		return nil, fmt.Errorf("daemon inspect failed: %w", err)
	}

	// 2. Build DAG and evaluate scores
	dag := analyzer.BuildDAG(snapshot)
	report := analyzer.EvaluateEngine(dag, snapshot, d.config.Policy, time.Now())

	// Update Prometheus cache metrics
	d.metrics.WarmCacheBytes.Set(float64(report.WarmCacheSizeBytes))

	// 3. Check disk usage watermark
	var diskUsageRatio float64
	usage, err := d.client.DiskUsage(ctx, types.DiskUsageOptions{})
	if err == nil && usage.LayersSize > 0 {
		// Calculate ratio from disk usage if available
		// Default to estimated ratio based on total image size if capacity is unspecified
		diskUsageRatio = float64(report.PruneCandidateSizeBytes) / float64(snapshot.TotalImageSizeBytes+1)
	} else if snapshot.TotalImageSizeBytes > 0 {
		diskUsageRatio = float64(report.PruneCandidateSizeBytes) / float64(snapshot.TotalImageSizeBytes)
	}
	d.metrics.DiskUsageRatio.Set(diskUsageRatio)

	// Threshold watermark trigger:
	// If MaxDiskUsagePercent <= 0, always prune candidates.
	// Otherwise, check if disk usage exceeds the threshold.
	shouldPrune := d.config.MaxDiskUsagePercent <= 0 || (diskUsageRatio*100.0 >= d.config.MaxDiskUsagePercent)

	plan := cleaner.BuildPrunePlan(report)
	if !shouldPrune {
		log.Printf("[Daemon] Disk usage ratio (%.2f%%) is below configured threshold (%.2f%%). Skipping prune.",
			diskUsageRatio*100.0, d.config.MaxDiskUsagePercent)
		return &cleaner.PruneReport{
			DryRun:             d.config.DryRun,
			WarmCachePreserved: plan.WarmCacheProtectedByt,
		}, nil
	}

	// 4. Execute selective pruning
	pruneReport, err := d.pruner.Execute(ctx, plan, d.config.DryRun)
	modeStr := "live"
	if d.config.DryRun {
		modeStr = "dry_run"
	}

	if err != nil {
		d.metrics.RunsTotal.WithLabelValues("error", modeStr).Inc()
		return nil, fmt.Errorf("prune execution failed: %w", err)
	}

	d.metrics.RunsTotal.WithLabelValues("success", modeStr).Inc()
	d.metrics.ReclaimedBytesTotal.Add(float64(pruneReport.ReclaimedBytes))
	d.metrics.PrunedImagesTotal.Add(float64(pruneReport.ImagesPruned))
	d.metrics.PrunedVolumesTotal.Add(float64(pruneReport.VolumesPruned))

	return pruneReport, nil
}

// Start launches the daemon service with Prometheus metrics and recurrent evaluation.
func (d *Daemon) Start(ctx context.Context) error {
	// Start Prometheus metrics server
	server := metrics.NewMetricsServer(d.config.MetricsAddr, d.metrics)
	serverErrChan := make(chan error, 1)

	go func() {
		if err := server.ListenAndServe(); err != nil && err.Error() != "http: Server closed" {
			serverErrChan <- err
		}
	}()

	log.Printf("[Daemon] Started PruneDocker daemon. Metrics listening on %s, check interval: %v",
		d.config.MetricsAddr, d.config.CheckInterval)

	// Run immediate initial check
	if _, err := d.RunOnce(ctx); err != nil {
		log.Printf("[Daemon] Initial run warning: %v", err)
	}

	ticker := time.NewTicker(d.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[Daemon] Shutdown signal received. Stopping daemon...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = metrics.ShutdownServer(shutdownCtx, server)
			return ctx.Err()

		case err := <-serverErrChan:
			return fmt.Errorf("metrics server failed: %w", err)

		case <-ticker.C:
			log.Println("[Daemon] Interval triggered. Checking Docker storage...")
			if _, err := d.RunOnce(ctx); err != nil {
				log.Printf("[Daemon] Interval run error: %v", err)
			}
		}
	}
}
