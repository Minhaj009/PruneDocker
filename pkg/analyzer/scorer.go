package analyzer

import (
	"math"
	"time"

	"github.com/prunedocker/prunedocker/pkg/docker"
)

// ItemCategory represents the categorization of an image or volume for cache preservation.
type ItemCategory string

const (
	CategoryActive         ItemCategory = "ACTIVE_IN_USE"
	CategoryWarmCache      ItemCategory = "WARM_CACHE_KEPT"
	CategoryPruneCandidate ItemCategory = "DEAD_LEAF_PRUNE"
	CategoryOrphanVolume   ItemCategory = "ORPHAN_VOLUME_PRUNE"
	CategoryNamedVolume    ItemCategory = "NAMED_VOLUME_KEPT"
)

// ScoringPolicy defines the tuning thresholds for cache preservation.
type ScoringPolicy struct {
	KeepRecent     time.Duration `json:"keep_recent"`     // e.g. 72h
	MinReuse       int           `json:"min_reuse"`       // e.g. 2
	ScoreThreshold float64       `json:"score_threshold"` // e.g. 0.5
}

// DefaultPolicy returns the recommended default scoring policy.
func DefaultPolicy() ScoringPolicy {
	return ScoringPolicy{
		KeepRecent:     72 * time.Hour,
		MinReuse:       2,
		ScoreThreshold: 0.5,
	}
}

// ScoredImage represents an analyzed image with its cache score and classification.
type ScoredImage struct {
	Metadata       docker.ImageMetadata `json:"metadata"`
	IsLeaf         bool                 `json:"is_leaf"`
	DependentCount int                  `json:"dependent_count"`
	MaxLayerReuse  int                  `json:"max_layer_reuse"`
	EffectiveReuse int                  `json:"effective_reuse"`
	WeightRecent   float64              `json:"weight_recent"`
	Score          float64              `json:"score"`
	Category       ItemCategory         `json:"category"`
	Reason         string               `json:"reason"`
}

// ScoredVolume represents an analyzed Docker volume.
type ScoredVolume struct {
	Metadata docker.VolumeMetadata `json:"metadata"`
	Category ItemCategory          `json:"category"`
	Reason   string                `json:"reason"`
}

// AnalysisReport contains the evaluated scores across all images and volumes.
type AnalysisReport struct {
	Images                  []ScoredImage  `json:"images"`
	Volumes                 []ScoredVolume `json:"volumes"`
	WarmCacheCount          int            `json:"warm_cache_count"`
	WarmCacheSizeBytes      int64          `json:"warm_cache_size_bytes"`
	PruneCandidateCount     int            `json:"prune_candidate_count"`
	PruneCandidateSizeBytes int64          `json:"prune_candidate_size_bytes"`
	OrphanVolumeCount       int            `json:"orphan_volume_count"`
	OrphanVolumeSizeBytes   int64          `json:"orphan_volume_size_bytes"`
	ActiveProtectedCount    int            `json:"active_protected_count"`
	ActiveProtectedBytes    int64          `json:"active_protected_bytes"`
}

// CalculateWeightRecent calculates time decay weight for cache recency.
func CalculateWeightRecent(created time.Time, now time.Time, keepRecent time.Duration) float64 {
	if created.IsZero() {
		return 0.1
	}

	age := now.Sub(created)
	if age <= 0 {
		return 1.0 // created in future or right now
	}

	if keepRecent <= 0 {
		keepRecent = 72 * time.Hour
	}

	// Within keepRecent window: smooth linear decay from 1.0 to 0.5
	if age <= keepRecent {
		ratio := float64(age) / float64(keepRecent)
		return 1.0 - (0.5 * ratio)
	}

	// Beyond keepRecent: exponential half-life decay
	extraIntervals := float64(age-keepRecent) / float64(keepRecent)
	decay := 0.5 * math.Pow(2, -extraIntervals)
	if decay < 0.01 {
		return 0.01
	}
	return decay
}

// CalculateScore computes the AST cache score: Score = (Ref_Count * Weight_Recent) / Size_MB
func CalculateScore(refCount int, weightRecent float64, sizeMB float64) float64 {
	effectiveSize := math.Max(sizeMB, 0.1) // prevent division by zero or NaN
	return (float64(refCount) * weightRecent) / effectiveSize
}

// EvaluateEngine applies the scoring policy to a dependency DAG and snapshot.
func EvaluateEngine(dag *DependencyDAG, snapshot *docker.EngineSnapshot, policy ScoringPolicy, now time.Time) *AnalysisReport {
	if policy.KeepRecent <= 0 {
		policy.KeepRecent = 72 * time.Hour
	}
	if policy.MinReuse <= 0 {
		policy.MinReuse = 2
	}
	if policy.ScoreThreshold <= 0 {
		policy.ScoreThreshold = 0.5
	}
	if now.IsZero() {
		now = time.Now()
	}

	report := &AnalysisReport{
		Images:  make([]ScoredImage, 0, len(snapshot.Images)),
		Volumes: make([]ScoredVolume, 0, len(snapshot.Volumes)),
	}

	// 1. Evaluate Images
	for _, img := range snapshot.Images {
		imgNode := dag.ImageMap[img.ID]
		isLeaf := true
		depCount := 0
		if imgNode != nil {
			isLeaf = imgNode.IsLeaf
			depCount = imgNode.DependentCount
		}

		// Determine top layer reuse: how many images utilize the top-most layer of this stage
		topLayerReuse := 1
		if len(img.Layers) > 0 {
			topLayerHash := img.Layers[len(img.Layers)-1]
			if lNode, ok := dag.LayerMap[topLayerHash]; ok {
				topLayerReuse = lNode.RefCount
			}
		}

		effectiveReuse := depCount + 1
		if topLayerReuse > effectiveReuse {
			effectiveReuse = topLayerReuse
		}

		weightRecent := CalculateWeightRecent(img.CreatedAt, now, policy.KeepRecent)
		score := CalculateScore(effectiveReuse, weightRecent, img.SizeMB)

		var category ItemCategory
		var reason string

		// Rule 1: Actively used by running/stopped container or tagged release
		if img.InUse {
			category = CategoryActive
			score = math.Inf(1)
			reason = "Protected: Currently in use by an active container"
			report.ActiveProtectedCount++
			report.ActiveProtectedBytes += img.SizeBytes
		} else if !img.IsDangling {
			// Tagged production/development image
			category = CategoryActive
			score = math.Inf(1)
			reason = "Protected: Explicitly tagged repository image"
			report.ActiveProtectedCount++
			report.ActiveProtectedBytes += img.SizeBytes
		} else if !isLeaf {
			// Intermediate parent image that other images depend on
			category = CategoryWarmCache
			reason = "Protected: Intermediate parent required by child image layers"
			report.WarmCacheCount++
			report.WarmCacheSizeBytes += img.SizeBytes
		} else if effectiveReuse >= policy.MinReuse {
			// Shared intermediate layer used across multiple stages
			category = CategoryWarmCache
			reason = "Protected: High SHA-256 layer reuse across builds"
			report.WarmCacheCount++
			report.WarmCacheSizeBytes += img.SizeBytes
		} else if now.Sub(img.CreatedAt) <= policy.KeepRecent {
			// Fresh build within keep-recent window
			category = CategoryWarmCache
			reason = "Protected: Warm build cache within keep-recent window"
			report.WarmCacheCount++
			report.WarmCacheSizeBytes += img.SizeBytes
		} else if score >= policy.ScoreThreshold {
			// High cache score
			category = CategoryWarmCache
			reason = "Protected: Cache utility score exceeds threshold"
			report.WarmCacheCount++
			report.WarmCacheSizeBytes += img.SizeBytes
		} else {
			// Dead intermediate leaf
			category = CategoryPruneCandidate
			reason = "Prune Candidate: Dead intermediate leaf with low reuse and expired cache"
			report.PruneCandidateCount++
			report.PruneCandidateSizeBytes += img.SizeBytes
		}

		report.Images = append(report.Images, ScoredImage{
			Metadata:       img,
			IsLeaf:         isLeaf,
			DependentCount: depCount,
			MaxLayerReuse:  topLayerReuse,
			EffectiveReuse: effectiveReuse,
			WeightRecent:   weightRecent,
			Score:          score,
			Category:       category,
			Reason:         reason,
		})
	}

	// 2. Evaluate Volumes
	for _, vol := range snapshot.Volumes {
		var category ItemCategory
		var reason string

		if vol.InUse {
			category = CategoryNamedVolume
			reason = "Protected: Volume mounted by an active container"
		} else if vol.IsOrphaned {
			category = CategoryOrphanVolume
			reason = "Prune Candidate: Orphaned anonymous 64-char volume not mounted by any container"
			report.OrphanVolumeCount++
			report.OrphanVolumeSizeBytes += vol.SizeBytes
		} else {
			category = CategoryNamedVolume
			reason = "Protected: Named volume preserved"
		}

		report.Volumes = append(report.Volumes, ScoredVolume{
			Metadata: vol,
			Category: category,
			Reason:   reason,
		})
	}

	return report
}
