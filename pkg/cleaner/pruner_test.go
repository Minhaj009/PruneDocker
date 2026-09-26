package cleaner

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/image"
	"github.com/prunedocker/prunedocker/pkg/analyzer"
	"github.com/prunedocker/prunedocker/pkg/docker"
)

func TestBuildPrunePlan(t *testing.T) {
	report := &analyzer.AnalysisReport{
		WarmCacheCount:     2,
		WarmCacheSizeBytes: 250 * 1024 * 1024,
		Images: []analyzer.ScoredImage{
			{
				Metadata: docker.ImageMetadata{ID: "img_candidate1", SizeBytes: 100 * 1024 * 1024},
				Category: analyzer.CategoryPruneCandidate,
			},
			{
				Metadata: docker.ImageMetadata{ID: "img_candidate2", SizeBytes: 200 * 1024 * 1024},
				Category: analyzer.CategoryPruneCandidate,
			},
			{
				Metadata: docker.ImageMetadata{ID: "img_warm", SizeBytes: 250 * 1024 * 1024},
				Category: analyzer.CategoryWarmCache,
			},
		},
		Volumes: []analyzer.ScoredVolume{
			{
				Metadata: docker.VolumeMetadata{Name: "orphan_vol", SizeBytes: 50 * 1024 * 1024},
				Category: analyzer.CategoryOrphanVolume,
			},
			{
				Metadata: docker.VolumeMetadata{Name: "saved_vol", SizeBytes: 80 * 1024 * 1024},
				Category: analyzer.CategoryNamedVolume,
			},
		},
	}

	plan := BuildPrunePlan(report)

	if len(plan.ImageTargets) != 2 {
		t.Fatalf("expected 2 image targets, got %d", len(plan.ImageTargets))
	}
	if len(plan.VolumeTargets) != 1 {
		t.Fatalf("expected 1 volume target, got %d", len(plan.VolumeTargets))
	}
	if plan.ProjectedImageBytes != 300*1024*1024 {
		t.Errorf("expected 300MB projected image bytes, got %d", plan.ProjectedImageBytes)
	}
	if plan.ProjectedVolumeBytes != 50*1024*1024 {
		t.Errorf("expected 50MB projected volume bytes, got %d", plan.ProjectedVolumeBytes)
	}
	if plan.ProjectedTotalBytes != 350*1024*1024 {
		t.Errorf("expected 350MB projected total bytes, got %d", plan.ProjectedTotalBytes)
	}
	if plan.WarmCacheProtectedByt != 250*1024*1024 {
		t.Errorf("expected 250MB warm cache protected, got %d", plan.WarmCacheProtectedByt)
	}
}

func TestPrunerExecuteDryRun(t *testing.T) {
	mock := docker.NewMockDockerClient()
	pruner := NewPruner(mock)

	plan := &PrunePlan{
		ImageTargets: []analyzer.ScoredImage{
			{Metadata: docker.ImageMetadata{ID: "dead_img1", SizeBytes: 100}},
		},
		VolumeTargets: []analyzer.ScoredVolume{
			{Metadata: docker.VolumeMetadata{Name: "orphan_vol1", SizeBytes: 200}},
		},
		WarmCacheProtectedByt: 500,
	}

	report, err := pruner.Execute(context.Background(), plan, true)
	if err != nil {
		t.Fatalf("unexpected error in dry-run: %v", err)
	}

	if !report.DryRun {
		t.Errorf("expected report.DryRun = true")
	}
	if report.ReclaimedBytes != 300 {
		t.Errorf("expected 300 bytes reclaimed projection, got %d", report.ReclaimedBytes)
	}
	// Verify mock client recorded ZERO actual deletions!
	if len(mock.RemovedImages) != 0 {
		t.Errorf("expected 0 removed images in dry-run, got %d", len(mock.RemovedImages))
	}
	if len(mock.RemovedVolumes) != 0 {
		t.Errorf("expected 0 removed volumes in dry-run, got %d", len(mock.RemovedVolumes))
	}
}

func TestPrunerExecuteLive(t *testing.T) {
	mock := docker.NewMockDockerClient()
	pruner := NewPruner(mock)

	plan := &PrunePlan{
		ImageTargets: []analyzer.ScoredImage{
			{Metadata: docker.ImageMetadata{ID: "dead_img1", SizeBytes: 150}},
			{Metadata: docker.ImageMetadata{ID: "dead_img2", SizeBytes: 250}},
		},
		VolumeTargets: []analyzer.ScoredVolume{
			{Metadata: docker.VolumeMetadata{Name: "orphan_vol1", SizeBytes: 300}},
		},
		WarmCacheProtectedByt: 1000,
	}

	report, err := pruner.Execute(context.Background(), plan, false)
	if err != nil {
		t.Fatalf("unexpected error in live run: %v", err)
	}

	if report.DryRun {
		t.Errorf("expected live run, got DryRun=true")
	}
	if report.ImagesPruned != 2 {
		t.Errorf("expected 2 images pruned, got %d", report.ImagesPruned)
	}
	if report.VolumesPruned != 1 {
		t.Errorf("expected 1 volume pruned, got %d", report.VolumesPruned)
	}
	if report.ReclaimedBytes != 700 {
		t.Errorf("expected 700 bytes reclaimed, got %d", report.ReclaimedBytes)
	}

	if len(mock.RemovedImages) != 2 {
		t.Errorf("expected 2 mock image removals, got %d", len(mock.RemovedImages))
	}
	if len(mock.RemovedVolumes) != 1 {
		t.Errorf("expected 1 mock volume removal, got %d", len(mock.RemovedVolumes))
	}
}

func TestPrunerExecuteErrors(t *testing.T) {
	mock := docker.NewMockDockerClient()
	// Simulate error when deleting image 1
	mock.ImageRemoveFunc = func(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error) {
		if imageID == "locked_img" {
			return nil, errors.New("conflict: unable to delete, container is using it")
		}
		return []image.DeleteResponse{{Deleted: imageID}}, nil
	}

	// Simulate error when deleting volume 1
	mock.VolumeRemoveFunc = func(ctx context.Context, volumeID string, force bool) error {
		if volumeID == "locked_vol" {
			return errors.New("volume is in use")
		}
		return nil
	}

	pruner := NewPruner(mock)
	plan := &PrunePlan{
		ImageTargets: []analyzer.ScoredImage{
			{Metadata: docker.ImageMetadata{ID: "locked_img", SizeBytes: 100}},
			{Metadata: docker.ImageMetadata{ID: "normal_img", SizeBytes: 200}},
		},
		VolumeTargets: []analyzer.ScoredVolume{
			{Metadata: docker.VolumeMetadata{Name: "locked_vol", SizeBytes: 50}},
			{Metadata: docker.VolumeMetadata{Name: "normal_vol", SizeBytes: 150}},
		},
	}

	report, err := pruner.Execute(context.Background(), plan, false)
	if err != nil {
		t.Fatalf("pruner should not fail entirely when individual items error: %v", err)
	}

	// normal_img and normal_vol should succeed!
	if report.ImagesPruned != 1 {
		t.Errorf("expected 1 image pruned, got %d", report.ImagesPruned)
	}
	if report.VolumesPruned != 1 {
		t.Errorf("expected 1 volume pruned, got %d", report.VolumesPruned)
	}
	if report.ReclaimedBytes != 350 { // 200 + 150
		t.Errorf("expected 350 bytes reclaimed, got %d", report.ReclaimedBytes)
	}
}
