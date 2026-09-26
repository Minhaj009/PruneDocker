package cleaner

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/prunedocker/prunedocker/pkg/analyzer"
	"github.com/prunedocker/prunedocker/pkg/docker"
)

// PrunePlan encapsulates the exact set of deletion targets and safety invariants.
type PrunePlan struct {
	ImageTargets          []analyzer.ScoredImage  `json:"image_targets"`
	VolumeTargets         []analyzer.ScoredVolume `json:"volume_targets"`
	ProjectedImageBytes   int64                   `json:"projected_image_bytes"`
	ProjectedVolumeBytes  int64                   `json:"projected_volume_bytes"`
	ProjectedTotalBytes   int64                   `json:"projected_total_bytes"`
	WarmCacheProtectedCnt int                     `json:"warm_cache_protected_cnt"`
	WarmCacheProtectedByt int64                   `json:"warm_cache_protected_byt"`
}

// PruneResult records the outcome of an individual item deletion.
type PruneResult struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // "image" or "volume"
	Success     bool   `json:"success"`
	FreedBytes  int64  `json:"freed_bytes"`
	Error       string `json:"error,omitempty"`
	SkippedNote string `json:"skipped_note,omitempty"`
}

// PruneReport provides a comprehensive summary of the pruning execution.
type PruneReport struct {
	DryRun             bool          `json:"dry_run"`
	StartedAt          time.Time     `json:"started_at"`
	CompletedAt        time.Time     `json:"completed_at"`
	Duration           time.Duration `json:"duration"`
	ImagesAttempted    int           `json:"images_attempted"`
	ImagesPruned       int           `json:"images_pruned"`
	VolumesAttempted   int           `json:"volumes_attempted"`
	VolumesPruned      int           `json:"volumes_pruned"`
	ReclaimedBytes     int64         `json:"reclaimed_bytes"`
	WarmCachePreserved int64         `json:"warm_cache_preserved_bytes"`
	Results            []PruneResult `json:"results"`
}

// BuildPrunePlan extracts the actionable deletion targets from an analysis report.
func BuildPrunePlan(report *analyzer.AnalysisReport) *PrunePlan {
	plan := &PrunePlan{
		ImageTargets:          make([]analyzer.ScoredImage, 0),
		VolumeTargets:         make([]analyzer.ScoredVolume, 0),
		WarmCacheProtectedCnt: report.WarmCacheCount,
		WarmCacheProtectedByt: report.WarmCacheSizeBytes,
	}

	for _, img := range report.Images {
		if img.Category == analyzer.CategoryPruneCandidate {
			plan.ImageTargets = append(plan.ImageTargets, img)
			plan.ProjectedImageBytes += img.Metadata.SizeBytes
		}
	}

	for _, vol := range report.Volumes {
		if vol.Category == analyzer.CategoryOrphanVolume {
			plan.VolumeTargets = append(plan.VolumeTargets, vol)
			plan.ProjectedVolumeBytes += vol.Metadata.SizeBytes
		}
	}

	plan.ProjectedTotalBytes = plan.ProjectedImageBytes + plan.ProjectedVolumeBytes
	return plan
}

// Pruner executes atomic pruning transactions.
type Pruner struct {
	client docker.DockerClient
}

// NewPruner creates a new Pruner instance.
func NewPruner(client docker.DockerClient) *Pruner {
	return &Pruner{client: client}
}

// Execute carries out the pruning plan safely with zero collateral damage.
func (p *Pruner) Execute(ctx context.Context, plan *PrunePlan, dryRun bool) (*PruneReport, error) {
	startTime := time.Now()
	report := &PruneReport{
		DryRun:             dryRun,
		StartedAt:          startTime,
		WarmCachePreserved: plan.WarmCacheProtectedByt,
		Results:            make([]PruneResult, 0, len(plan.ImageTargets)+len(plan.VolumeTargets)),
	}

	if dryRun {
		// Dry run mode: calculate projections without calling Docker delete APIs
		for _, img := range plan.ImageTargets {
			report.ImagesAttempted++
			report.ImagesPruned++
			report.ReclaimedBytes += img.Metadata.SizeBytes
			report.Results = append(report.Results, PruneResult{
				ID:          img.Metadata.ID,
				Type:        "image",
				Success:     true,
				FreedBytes:  img.Metadata.SizeBytes,
				SkippedNote: "DRY-RUN (Simulated deletion)",
			})
		}

		for _, vol := range plan.VolumeTargets {
			report.VolumesAttempted++
			report.VolumesPruned++
			report.ReclaimedBytes += vol.Metadata.SizeBytes
			report.Results = append(report.Results, PruneResult{
				ID:          vol.Metadata.Name,
				Type:        "volume",
				Success:     true,
				FreedBytes:  vol.Metadata.SizeBytes,
				SkippedNote: "DRY-RUN (Simulated deletion)",
			})
		}

		report.CompletedAt = time.Now()
		report.Duration = report.CompletedAt.Sub(startTime)
		return report, nil
	}

	// Live execution:
	// 1. Delete dead leaf images with PruneChildren: false to strictly preserve parent caches
	for _, img := range plan.ImageTargets {
		report.ImagesAttempted++
		opts := image.RemoveOptions{
			Force:         false,
			PruneChildren: false, // Critical: never cascade delete shared parent layers!
		}

		delResps, err := p.client.ImageRemove(ctx, img.Metadata.ID, opts)
		if err != nil {
			report.Results = append(report.Results, PruneResult{
				ID:      img.Metadata.ID,
				Type:    "image",
				Success: false,
				Error:   err.Error(),
			})
			continue
		}

		// Calculate reclaimed space
		freed := img.Metadata.SizeBytes
		report.ImagesPruned++
		report.ReclaimedBytes += freed
		report.Results = append(report.Results, PruneResult{
			ID:         img.Metadata.ID,
			Type:       "image",
			Success:    true,
			FreedBytes: freed,
			SkippedNote: fmt.Sprintf("Deleted %d items", len(delResps)),
		})
	}

	// 2. Delete orphaned anonymous volumes
	for _, vol := range plan.VolumeTargets {
		report.VolumesAttempted++
		err := p.client.VolumeRemove(ctx, vol.Metadata.Name, false)
		if err != nil {
			report.Results = append(report.Results, PruneResult{
				ID:      vol.Metadata.Name,
				Type:    "volume",
				Success: false,
				Error:   err.Error(),
			})
			continue
		}

		freed := vol.Metadata.SizeBytes
		report.VolumesPruned++
		report.ReclaimedBytes += freed
		report.Results = append(report.Results, PruneResult{
			ID:         vol.Metadata.Name,
			Type:       "volume",
			Success:    true,
			FreedBytes: freed,
		})
	}

	report.CompletedAt = time.Now()
	report.Duration = report.CompletedAt.Sub(startTime)
	return report, nil
}
