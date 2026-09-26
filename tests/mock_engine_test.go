package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/prunedocker/prunedocker/pkg/analyzer"
	"github.com/prunedocker/prunedocker/pkg/cleaner"
	"github.com/prunedocker/prunedocker/pkg/docker"
)

// SimulatedDockerEngine implements an in-memory HTTP server mimicking the Docker Engine REST API.
type SimulatedDockerEngine struct {
	mu             sync.Mutex
	Images         map[string]types.ImageInspect
	Summaries      []image.Summary
	Containers     []types.Container
	Volumes        []*volume.Volume
	DeletedImages  []string
	DeletedVolumes []string
	Server         *httptest.Server
}

func NewSimulatedDockerEngine() *SimulatedDockerEngine {
	engine := &SimulatedDockerEngine{
		Images:         make(map[string]types.ImageInspect),
		Summaries:      make([]image.Summary, 0),
		Containers:     make([]types.Container, 0),
		Volumes:        make([]*volume.Volume, 0),
		DeletedImages:  make([]string, 0),
		DeletedVolumes: make([]string, 0),
	}

	mux := http.NewServeMux()

	// 1. _ping
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("API-Version", "1.45")
		w.Header().Set("Docker-Experimental", "false")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 2. /version
	handleVersion := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("API-Version", "1.45")
		json.NewEncoder(w).Encode(types.Version{
			APIVersion: "1.45",
			Version:    "27.1.1",
			Os:         "linux",
		})
	}
	mux.HandleFunc("/version", handleVersion)
	mux.HandleFunc("/v1.45/version", handleVersion)

	// 3. /containers/json
	handleContainers := func(w http.ResponseWriter, r *http.Request) {
		engine.mu.Lock()
		defer engine.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(engine.Containers)
	}
	mux.HandleFunc("/containers/json", handleContainers)
	mux.HandleFunc("/v1.45/containers/json", handleContainers)

	// 4. /images/json
	handleImages := func(w http.ResponseWriter, r *http.Request) {
		engine.mu.Lock()
		defer engine.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(engine.Summaries)
	}
	mux.HandleFunc("/images/json", handleImages)
	mux.HandleFunc("/v1.45/images/json", handleImages)

	// 5. /images/{id}/json and DELETE /images/{id}
	handleImageItem := func(w http.ResponseWriter, r *http.Request) {
		engine.mu.Lock()
		defer engine.mu.Unlock()

		path := r.URL.Path
		// Strip prefixes
		path = strings.TrimPrefix(path, "/v1.45/images/")
		path = strings.TrimPrefix(path, "/images/")

		if r.Method == http.MethodGet && strings.HasSuffix(path, "/json") {
			imageID := strings.TrimSuffix(path, "/json")
			inspect, ok := engine.Images[imageID]
			if !ok {
				http.Error(w, fmt.Sprintf("No such image: %s", imageID), http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(inspect)
			return
		}

		if r.Method == http.MethodDelete {
			imageID := path
			engine.DeletedImages = append(engine.DeletedImages, imageID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]image.DeleteResponse{{Deleted: imageID}})
			return
		}

		http.NotFound(w, r)
	}
	mux.HandleFunc("/images/", handleImageItem)
	mux.HandleFunc("/v1.45/images/", handleImageItem)

	// 6. /volumes and DELETE /volumes/{name}
	handleVolumes := func(w http.ResponseWriter, r *http.Request) {
		engine.mu.Lock()
		defer engine.mu.Unlock()

		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(volume.ListResponse{Volumes: engine.Volumes})
			return
		}
		http.NotFound(w, r)
	}
	mux.HandleFunc("/volumes", handleVolumes)
	mux.HandleFunc("/v1.45/volumes", handleVolumes)

	handleVolumeDelete := func(w http.ResponseWriter, r *http.Request) {
		engine.mu.Lock()
		defer engine.mu.Unlock()

		if r.Method == http.MethodDelete {
			volName := strings.TrimPrefix(r.URL.Path, "/v1.45/volumes/")
			volName = strings.TrimPrefix(volName, "/volumes/")
			engine.DeletedVolumes = append(engine.DeletedVolumes, volName)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}
	mux.HandleFunc("/volumes/", handleVolumeDelete)
	mux.HandleFunc("/v1.45/volumes/", handleVolumeDelete)

	engine.Server = httptest.NewServer(mux)
	return engine
}

func (e *SimulatedDockerEngine) Close() {
	e.Server.Close()
}

func TestEndToEndMultiStageBuildPruning(t *testing.T) {
	engine := NewSimulatedDockerEngine()
	defer engine.Close()

	now := time.Now().UTC()

	// Scenario:
	// Layer A: Base OS (Ubuntu/Alpine) -> shared by all
	// Layer B: Go SDK / Toolchain -> shared by Build 1 and Build 2
	// Layer C1: Build 1 intermediate dependency cache -> recent (warm cache)
	// Layer D1: Build 1 final production binary (tagged: "app:prod", running container)
	// Layer C2: Build 2 abandoned branch test -> 3 weeks old (dead leaf candidate)
	// Volume 1: Anonymous orphaned volume
	// Volume 2: Postgres data volume mounted by container

	layerA := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	layerB := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	layerC1 := "sha256:c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1"
	layerD1 := "sha256:d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1"
	layerC2 := "sha256:c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2"

	imgBaseID := "sha256:1111000000000000000000000000000000000000000000000000000000000001"
	imgWarmStageID := "sha256:1111000000000000000000000000000000000000000000000000000000000002"
	imgProdID := "sha256:1111000000000000000000000000000000000000000000000000000000000003"
	imgDeadID := "sha256:1111000000000000000000000000000000000000000000000000000000000004"

	anonVolName := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	dbVolName := "postgres_production_data"

	// 1. Populate Mock Data
	engine.mu.Lock()

	// Base image (tagged alpine)
	engine.Summaries = append(engine.Summaries, image.Summary{
		ID:       imgBaseID,
		RepoTags: []string{"alpine:3.19"},
		Size:     10 * 1024 * 1024,
		Created:  now.Add(-500 * time.Hour).Unix(),
	})
	engine.Images[imgBaseID] = types.ImageInspect{
		ID:      imgBaseID,
		Size:    10 * 1024 * 1024,
		Created: now.Add(-500 * time.Hour).Format(time.RFC3339),
		RootFS:  types.RootFS{Layers: []string{layerA}},
	}

	// Warm intermediate stage (shared cache)
	engine.Summaries = append(engine.Summaries, image.Summary{
		ID:       imgWarmStageID,
		RepoTags: []string{"<none>:<none>"},
		Size:     80 * 1024 * 1024,
		Created:  now.Add(-12 * time.Hour).Unix(), // created 12h ago -> WARM!
	})
	engine.Images[imgWarmStageID] = types.ImageInspect{
		ID:      imgWarmStageID,
		Size:    80 * 1024 * 1024,
		Created: now.Add(-12 * time.Hour).Format(time.RFC3339),
		RootFS:  types.RootFS{Layers: []string{layerA, layerB, layerC1}},
	}

	// Production tagged image
	engine.Summaries = append(engine.Summaries, image.Summary{
		ID:       imgProdID,
		RepoTags: []string{"myapp:v1.0"},
		Size:     150 * 1024 * 1024,
		Created:  now.Add(-6 * time.Hour).Unix(),
	})
	engine.Images[imgProdID] = types.ImageInspect{
		ID:      imgProdID,
		Size:    150 * 1024 * 1024,
		Created: now.Add(-6 * time.Hour).Format(time.RFC3339),
		RootFS:  types.RootFS{Layers: []string{layerA, layerB, layerC1, layerD1}},
	}

	// Dead abandoned leaf image
	engine.Summaries = append(engine.Summaries, image.Summary{
		ID:       imgDeadID,
		RepoTags: []string{"<none>:<none>"},
		Size:     500 * 1024 * 1024, // 500 MB dead blob
		Created:  now.Add(-600 * time.Hour).Unix(), // old
	})
	engine.Images[imgDeadID] = types.ImageInspect{
		ID:      imgDeadID,
		Size:    500 * 1024 * 1024,
		Created: now.Add(-600 * time.Hour).Format(time.RFC3339),
		RootFS:  types.RootFS{Layers: []string{layerA, layerB, layerC2}},
	}

	// Active container running myapp and mounting db volume
	engine.Containers = append(engine.Containers, types.Container{
		ID:      "c_prod",
		ImageID: imgProdID,
		Image:   "myapp:v1.0",
		Mounts: []types.MountPoint{
			{Name: dbVolName},
		},
	})

	// Volumes
	engine.Volumes = append(engine.Volumes,
		&volume.Volume{
			Name:   anonVolName,
			Driver: "local",
			UsageData: &volume.UsageData{
				Size: 50 * 1024 * 1024,
			},
		},
		&volume.Volume{
			Name:   dbVolName,
			Driver: "local",
			UsageData: &volume.UsageData{
				Size: 200 * 1024 * 1024,
			},
		},
	)
	engine.mu.Unlock()

	// 2. Connect PruneDocker Client to the Simulated Engine URL
	client, err := docker.NewEngineClient(engine.Server.URL)
	if err != nil {
		t.Fatalf("failed to create client for test server: %v", err)
	}
	defer client.Close()

	// 3. Inspect Engine
	ctx := context.Background()
	snapshot, err := docker.InspectEngine(ctx, client)
	if err != nil {
		t.Fatalf("InspectEngine failed over HTTP: %v", err)
	}

	if len(snapshot.Images) != 4 {
		t.Fatalf("expected 4 images, got %d", len(snapshot.Images))
	}
	if len(snapshot.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(snapshot.Volumes))
	}

	// 4. Build DAG
	dag := analyzer.BuildDAG(snapshot)
	if dag.TotalImages != 4 {
		t.Errorf("DAG total images: expected 4, got %d", dag.TotalImages)
	}

	// Layer A and B must have high reference counts (used by warm, prod, dead)
	if dag.LayerMap[layerA] == nil || dag.LayerMap[layerA].RefCount != 4 {
		t.Errorf("layerA should be referenced by all 4 images, got %v", dag.LayerMap[layerA])
	}
	if dag.LayerMap[layerB] == nil || dag.LayerMap[layerB].RefCount != 3 {
		t.Errorf("layerB should be referenced by 3 images, got %v", dag.LayerMap[layerB])
	}

	// 5. Evaluate Scorer
	policy := analyzer.DefaultPolicy()
	report := analyzer.EvaluateEngine(dag, snapshot, policy, now)

	// Verify Category Protections:
	// - imgBaseID: ACTIVE (tagged)
	// - imgProdID: ACTIVE (in use by container & tagged)
	// - imgWarmStageID: WARM_CACHE_KEPT (recent build + high layer reuse)
	// - imgDeadID: DEAD_LEAF_PRUNE (dangling, old, low score)
	for _, img := range report.Images {
		switch img.Metadata.ID {
		case imgBaseID:
			if img.Category != analyzer.CategoryActive {
				t.Errorf("base image alpine should be CategoryActive, got %s", img.Category)
			}
		case imgProdID:
			if img.Category != analyzer.CategoryActive {
				t.Errorf("production image should be CategoryActive, got %s", img.Category)
			}
		case imgWarmStageID:
			if img.Category != analyzer.CategoryWarmCache {
				t.Errorf("warm stage should be CategoryWarmCache, got %s (%s)", img.Category, img.Reason)
			}
		case imgDeadID:
			if img.Category != analyzer.CategoryPruneCandidate {
				t.Errorf("dead leaf should be CategoryPruneCandidate, got %s", img.Category)
			}
		}
	}

	// Verify Volumes:
	for _, vol := range report.Volumes {
		switch vol.Metadata.Name {
		case anonVolName:
			if vol.Category != analyzer.CategoryOrphanVolume {
				t.Errorf("anonymous volume should be CategoryOrphanVolume, got %s", vol.Category)
			}
		case dbVolName:
			if vol.Category != analyzer.CategoryNamedVolume {
				t.Errorf("db volume should be CategoryNamedVolume, got %s", vol.Category)
			}
		}
	}

	// 6. Build and Execute Prune Plan
	plan := cleaner.BuildPrunePlan(report)
	if len(plan.ImageTargets) != 1 || plan.ImageTargets[0].Metadata.ID != imgDeadID {
		t.Fatalf("expected exactly 1 image target (imgDeadID), got %v", plan.ImageTargets)
	}
	if len(plan.VolumeTargets) != 1 || plan.VolumeTargets[0].Metadata.Name != anonVolName {
		t.Fatalf("expected exactly 1 volume target (anonVolName), got %v", plan.VolumeTargets)
	}

	pruner := cleaner.NewPruner(client)
	pruneReport, err := pruner.Execute(ctx, plan, false)
	if err != nil {
		t.Fatalf("prune execution failed: %v", err)
	}

	if pruneReport.ImagesPruned != 1 {
		t.Errorf("expected 1 image pruned, got %d", pruneReport.ImagesPruned)
	}
	if pruneReport.VolumesPruned != 1 {
		t.Errorf("expected 1 volume pruned, got %d", pruneReport.VolumesPruned)
	}

	// 7. Assert that ONLY imgDeadID and anonVolName were sent for deletion!
	engine.mu.Lock()
	defer engine.mu.Unlock()

	if len(engine.DeletedImages) != 1 || engine.DeletedImages[0] != imgDeadID {
		t.Errorf("server received unexpected deleted images: %v", engine.DeletedImages)
	}
	if len(engine.DeletedVolumes) != 1 || engine.DeletedVolumes[0] != anonVolName {
		t.Errorf("server received unexpected deleted volumes: %v", engine.DeletedVolumes)
	}
}
