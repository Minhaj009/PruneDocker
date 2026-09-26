package analyzer

import (
	"math"
	"testing"
	"time"

	"github.com/prunedocker/prunedocker/pkg/docker"
)

func TestCalculateWeightRecent(t *testing.T) {
	now := time.Now()
	keepRecent := 72 * time.Hour

	// Future time or now -> 1.0
	if w := CalculateWeightRecent(now.Add(1*time.Hour), now, keepRecent); w != 1.0 {
		t.Errorf("expected 1.0 for future time, got %f", w)
	}
	if w := CalculateWeightRecent(now, now, keepRecent); w != 1.0 {
		t.Errorf("expected 1.0 for now, got %f", w)
	}

	// 36 hours old (halfway) -> 1.0 - 0.5 * 0.5 = 0.75
	wHalf := CalculateWeightRecent(now.Add(-36*time.Hour), now, keepRecent)
	if math.Abs(wHalf-0.75) > 0.01 {
		t.Errorf("expected ~0.75 for 36h age, got %f", wHalf)
	}

	// Exactly 72 hours old -> 0.5
	w72 := CalculateWeightRecent(now.Add(-72*time.Hour), now, keepRecent)
	if math.Abs(w72-0.5) > 0.01 {
		t.Errorf("expected ~0.5 for 72h age, got %f", w72)
	}

	// 144 hours old (2x keepRecent) -> 0.5 * 2^(-1) = 0.25
	w144 := CalculateWeightRecent(now.Add(-144*time.Hour), now, keepRecent)
	if math.Abs(w144-0.25) > 0.01 {
		t.Errorf("expected ~0.25 for 144h age, got %f", w144)
	}

	// Zero time fallback
	if wZero := CalculateWeightRecent(time.Time{}, now, keepRecent); wZero != 0.1 {
		t.Errorf("expected 0.1 for zero time, got %f", wZero)
	}
}

func TestCalculateScore(t *testing.T) {
	// Score = (Ref_Count * Weight_Recent) / Size_MB
	// High reuse (5), recent (1.0), 10 MB -> (5 * 1.0) / 10 = 0.5
	s1 := CalculateScore(5, 1.0, 10.0)
	if math.Abs(s1-0.5) > 0.001 {
		t.Errorf("expected score 0.5, got %f", s1)
	}

	// Low reuse (1), old (0.1), 500 MB -> (1 * 0.1) / 500 = 0.0002
	s2 := CalculateScore(1, 0.1, 500.0)
	if s2 >= 0.001 {
		t.Errorf("expected near-zero score for large dead layer, got %f", s2)
	}

	// Zero size defense: should use min 0.1 MB and not divide by zero or panic
	sZero := CalculateScore(1, 1.0, 0.0)
	if math.IsNaN(sZero) || math.IsInf(sZero, 0) || sZero <= 0 {
		t.Errorf("expected valid non-zero score with zero-size defense, got %f", sZero)
	}
}

func TestEvaluateEngine(t *testing.T) {
	now := time.Now()
	policy := DefaultPolicy()

	snapshot := &docker.EngineSnapshot{
		Images: []docker.ImageMetadata{
			{
				ID:         "img_active",
				ShortID:    "img_active",
				RepoTags:   []string{"app:prod"},
				InUse:      true,
				SizeBytes:  200 * 1024 * 1024,
				SizeMB:     200,
				CreatedAt:  now.Add(-200 * time.Hour),
				Layers:     []string{"sha256:layer1"},
				IsDangling: false,
			},
			{
				ID:         "img_tagged_unused",
				ShortID:    "img_tagged",
				RepoTags:   []string{"nginx:alpine"},
				InUse:      false,
				SizeBytes:  50 * 1024 * 1024,
				SizeMB:     50,
				CreatedAt:  now.Add(-200 * time.Hour),
				Layers:     []string{"sha256:layer2"},
				IsDangling: false,
			},
			{
				ID:         "img_warm_recent",
				ShortID:    "img_warm",
				RepoTags:   []string{"<none>:<none>"},
				InUse:      false,
				SizeBytes:  100 * 1024 * 1024,
				SizeMB:     100,
				CreatedAt:  now.Add(-10 * time.Hour), // within 72h keep-recent
				Layers:     []string{"sha256:layer3"},
				IsDangling: true,
			},
			{
				ID:         "img_dead_leaf",
				ShortID:    "img_dead",
				RepoTags:   []string{"<none>:<none>"},
				InUse:      false,
				SizeBytes:  800 * 1024 * 1024,
				SizeMB:     800,
				CreatedAt:  now.Add(-500 * time.Hour), // 3 weeks old
				Layers:     []string{"sha256:layer4"},
				IsDangling: true,
			},
		},
		Volumes: []docker.VolumeMetadata{
			{
				Name:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				SizeBytes:   50 * 1024 * 1024,
				InUse:       false,
				IsAnonymous: true,
				IsOrphaned:  true,
			},
			{
				Name:        "database_storage",
				SizeBytes:   100 * 1024 * 1024,
				InUse:       true,
				IsAnonymous: false,
				IsOrphaned:  false,
			},
		},
	}

	dag := BuildDAG(snapshot)
	report := EvaluateEngine(dag, snapshot, policy, now)

	// Assert image classifications
	if report.ActiveProtectedCount != 2 {
		t.Errorf("expected 2 active/tagged protected images, got %d", report.ActiveProtectedCount)
	}
	if report.WarmCacheCount != 1 {
		t.Errorf("expected 1 warm cache image, got %d", report.WarmCacheCount)
	}
	if report.PruneCandidateCount != 1 {
		t.Errorf("expected 1 dead leaf prune candidate, got %d", report.PruneCandidateCount)
	}

	// Verify the dead candidate is indeed img_dead_leaf
	var deadCandidate *ScoredImage
	for i := range report.Images {
		if report.Images[i].Metadata.ID == "img_dead_leaf" {
			deadCandidate = &report.Images[i]
			break
		}
	}
	if deadCandidate == nil || deadCandidate.Category != CategoryPruneCandidate {
		t.Fatalf("expected img_dead_leaf to be classified as CategoryPruneCandidate")
	}

	// Assert volume classifications
	if report.OrphanVolumeCount != 1 {
		t.Errorf("expected 1 orphaned volume, got %d", report.OrphanVolumeCount)
	}
	if report.Volumes[0].Category != CategoryOrphanVolume {
		t.Errorf("expected volume 0 to be CategoryOrphanVolume, got %s", report.Volumes[0].Category)
	}
	if report.Volumes[1].Category != CategoryNamedVolume {
		t.Errorf("expected volume 1 to be CategoryNamedVolume, got %s", report.Volumes[1].Category)
	}
}
