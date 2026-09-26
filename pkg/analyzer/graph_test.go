package analyzer

import (
	"testing"
	"time"

	"github.com/prunedocker/prunedocker/pkg/docker"
)

func TestBuildDAGMultiStage(t *testing.T) {
	// Simulate multi-stage build:
	// Base layer: L0, L1 (shared by img1 and img2)
	// img1: [L0, L1, L_app1] (parent image / stage)
	// img2: [L0, L1, L_app2] (another build)
	// img3: [L0, L1, L_app1, L_final] (final image built on img1's layers)

	now := time.Now()
	snapshot := &docker.EngineSnapshot{
		Images: []docker.ImageMetadata{
			{
				ID:        "img1",
				ShortID:   "img1",
				Layers:    []string{"sha256:L0", "sha256:L1", "sha256:L_app1"},
				SizeBytes: 100 * 1024 * 1024,
				CreatedAt: now,
			},
			{
				ID:        "img2",
				ShortID:   "img2",
				Layers:    []string{"sha256:L0", "sha256:L1", "sha256:L_app2"},
				SizeBytes: 120 * 1024 * 1024,
				CreatedAt: now,
			},
			{
				ID:        "img3",
				ShortID:   "img3",
				Layers:    []string{"sha256:L0", "sha256:L1", "sha256:L_app1", "sha256:L_final"},
				SizeBytes: 150 * 1024 * 1024,
				CreatedAt: now,
			},
		},
	}

	dag := BuildDAG(snapshot)

	if dag.TotalImages != 3 {
		t.Fatalf("expected 3 images, got %d", dag.TotalImages)
	}

	// Total unique layers: L0, L1, L_app1, L_app2, L_final = 5 layers
	if dag.TotalLayers != 5 {
		t.Fatalf("expected 5 unique layers, got %d", dag.TotalLayers)
	}

	// L0 and L1 must have RefCount = 3 (used by img1, img2, img3)
	l0 := dag.LayerMap["sha256:L0"]
	if l0 == nil || l0.RefCount != 3 {
		t.Errorf("expected L0 RefCount=3, got %v", l0)
	}
	l1 := dag.LayerMap["sha256:L1"]
	if l1 == nil || l1.RefCount != 3 {
		t.Errorf("expected L1 RefCount=3, got %v", l1)
	}

	// L_app1 used by img1 and img3 -> RefCount = 2
	lApp1 := dag.LayerMap["sha256:L_app1"]
	if lApp1 == nil || lApp1.RefCount != 2 {
		t.Errorf("expected L_app1 RefCount=2, got %v", lApp1)
	}

	// L_final used only by img3 -> RefCount = 1
	lFinal := dag.LayerMap["sha256:L_final"]
	if lFinal == nil || lFinal.RefCount != 1 {
		t.Errorf("expected L_final RefCount=1, got %v", lFinal)
	}

	// img1 is a prefix of img3, so img1 MUST NOT be a leaf
	nodeImg1 := dag.ImageMap["img1"]
	if nodeImg1.IsLeaf {
		t.Errorf("img1 should NOT be a leaf because img3 builds on its layers")
	}
	if nodeImg1.DependentCount != 1 {
		t.Errorf("img1 should have 1 dependent (img3), got %d", nodeImg1.DependentCount)
	}

	// img2 and img3 are leaves
	nodeImg2 := dag.ImageMap["img2"]
	if !nodeImg2.IsLeaf {
		t.Errorf("img2 should be a leaf")
	}

	nodeImg3 := dag.ImageMap["img3"]
	if !nodeImg3.IsLeaf {
		t.Errorf("img3 should be a leaf")
	}

	// Shared prefix length between img1 and img2 should be 2 (L0, L1)
	prefix12 := dag.GetSharedPrefixLength("img1", "img2")
	if prefix12 != 2 {
		t.Errorf("expected shared prefix length 2, got %d", prefix12)
	}

	// Shared prefix between img1 and img3 should be 3 (L0, L1, L_app1)
	prefix13 := dag.GetSharedPrefixLength("img1", "img3")
	if prefix13 != 3 {
		t.Errorf("expected shared prefix length 3, got %d", prefix13)
	}

	// Max reuse layer should be L0 or L1 with count 3
	maxLayer := dag.GetMaxReuseLayer()
	if maxLayer == nil || maxLayer.RefCount != 3 {
		t.Errorf("expected max reuse layer with count 3, got %v", maxLayer)
	}
}

func TestBuildDAGEmpty(t *testing.T) {
	dag := BuildDAG(nil)
	if dag.TotalImages != 0 || dag.TotalLayers != 0 {
		t.Errorf("expected empty DAG from nil snapshot")
	}

	dag2 := BuildDAG(&docker.EngineSnapshot{})
	if dag2.TotalImages != 0 || dag2.TotalLayers != 0 {
		t.Errorf("expected empty DAG from empty snapshot")
	}
}
