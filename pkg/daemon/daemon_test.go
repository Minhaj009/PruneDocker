package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/prunedocker/prunedocker/pkg/analyzer"
	"github.com/prunedocker/prunedocker/pkg/docker"
)

func setupMockDaemon(maxUsage float64, dryRun bool) (*Daemon, *docker.MockDockerClient) {
	mock := docker.NewMockDockerClient()
	now := time.Now()

	// Provide 1 dead leaf image (candidate) and 1 warm cache image
	mock.ImageListFunc = func(ctx context.Context, options image.ListOptions) ([]image.Summary, error) {
		return []image.Summary{
			{
				ID:       "dead_img",
				RepoTags: []string{"<none>:<none>"},
				Size:     200 * 1024 * 1024,
				Created:  now.Add(-200 * time.Hour).Unix(),
			},
			{
				ID:       "warm_img",
				RepoTags: []string{"<none>:<none>"},
				Size:     100 * 1024 * 1024,
				Created:  now.Add(-5 * time.Hour).Unix(), // recent -> warm
			},
		}, nil
	}

	mock.VolumeListFunc = func(ctx context.Context, filter volume.ListOptions) (volume.ListResponse, error) {
		return volume.ListResponse{
			Volumes: []*volume.Volume{
				{
					Name:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Driver: "local",
					UsageData: &volume.UsageData{
						Size: 50 * 1024 * 1024,
					},
				},
			},
		}, nil
	}

	cfg := Config{
		MaxDiskUsagePercent: maxUsage,
		CheckInterval:       50 * time.Millisecond,
		MetricsAddr:         ":0", // ephemeral port
		DryRun:              dryRun,
		Policy:              analyzer.DefaultPolicy(),
	}

	d := NewDaemon(mock, cfg)
	return d, mock
}

func TestDaemonRunOnceAlwaysPrune(t *testing.T) {
	d, mock := setupMockDaemon(0, false) // 0 means always prune

	report, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error in RunOnce: %v", err)
	}

	if report.ImagesPruned != 1 {
		t.Errorf("expected 1 image pruned, got %d", report.ImagesPruned)
	}
	if report.VolumesPruned != 1 {
		t.Errorf("expected 1 volume pruned, got %d", report.VolumesPruned)
	}
	if len(mock.RemovedImages) != 1 {
		t.Errorf("expected 1 mock remove call, got %d", len(mock.RemovedImages))
	}
}

func TestDaemonRunOnceWatermarkSkip(t *testing.T) {
	d, mock := setupMockDaemon(99.0, false) // 99% usage threshold

	report, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error in RunOnce: %v", err)
	}

	if report.ImagesPruned != 0 {
		t.Errorf("expected 0 images pruned due to watermark skip, got %d", report.ImagesPruned)
	}
	if len(mock.RemovedImages) != 0 {
		t.Errorf("expected 0 removals called, got %d", len(mock.RemovedImages))
	}
}

func TestDaemonRunOnceDryRun(t *testing.T) {
	d, mock := setupMockDaemon(0, true) // Dry run

	report, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error in dry-run: %v", err)
	}

	if !report.DryRun {
		t.Errorf("expected report.DryRun = true")
	}
	if len(mock.RemovedImages) != 0 {
		t.Errorf("expected 0 actual removals in dry run, got %d", len(mock.RemovedImages))
	}
}

func TestDaemonStartAndStop(t *testing.T) {
	d, _ := setupMockDaemon(0, true)

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- d.Start(ctx)
	}()

	// Wait briefly then cancel
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errChan:
		if err != nil && err != context.Canceled {
			t.Fatalf("unexpected daemon exit error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("daemon did not shutdown cleanly within timeout")
	}
}
