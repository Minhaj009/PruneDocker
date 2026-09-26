package docker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
)

func TestIsAnonymousVolume(t *testing.T) {
	cases := []struct {
		name     string
		expected bool
	}{
		{"4f2b1a3d9e8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e1d0c9b8a7f6e5d4c3b2a", true},
		{"4F2B1A3D9E8C7B6A5F4E3D2C1B0A9F8E7D6C5B4A3F2E1D0C9B8A7F6E5D4C3B2A", true}, // case insensitive
		{"postgres_data", false},
		{"redis-vol", false},
		{"4f2b1a3d", false}, // too short
		{"4f2b1a3d9e8c7b6a5f4e3d2c1b0a9f8e7d6c5b4a3f2e1d0c9b8a7f6e5d4c3b2z", false}, // 'z' is invalid hex
		{"", false},
	}

	for _, c := range cases {
		result := IsAnonymousVolume(c.name)
		if result != c.expected {
			t.Errorf("IsAnonymousVolume(%q) = %v; want %v", c.name, result, c.expected)
		}
	}
}

func TestInspectEngine(t *testing.T) {
	mock := NewMockDockerClient()

	now := time.Now().UTC()
	img1ID := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	img2ID := "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	img3ID := "sha256:3333333333333333333333333333333333333333333333333333333333333333"

	anonVolName := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	namedVolName := "db_storage"

	// Mock containers: Container 1 uses img1 and mounts namedVolName
	mock.ContainerListFunc = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
		return []types.Container{
			{
				ID:      "c1",
				ImageID: img1ID,
				Mounts: []types.MountPoint{
					{Name: namedVolName},
				},
			},
		}, nil
	}

	// Mock images:
	// img1: tagged, in use by c1
	// img2: dangling, not in use
	// img3: tagged, not in use
	mock.ImageListFunc = func(ctx context.Context, options image.ListOptions) ([]image.Summary, error) {
		return []image.Summary{
			{
				ID:       img1ID,
				RepoTags: []string{"myapp:v1"},
				Size:     100 * 1024 * 1024,
				Created:  now.Unix(),
			},
			{
				ID:       img2ID,
				RepoTags: []string{"<none>:<none>"},
				Size:     50 * 1024 * 1024,
				Created:  now.Add(-48 * time.Hour).Unix(),
			},
			{
				ID:       img3ID,
				RepoTags: []string{"redis:alpine"},
				Size:     30 * 1024 * 1024,
				Created:  now.Add(-24 * time.Hour).Unix(),
			},
		}, nil
	}

	mock.ImageInspectWithRawFunc = func(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
		switch imageID {
		case img1ID:
			return types.ImageInspect{
				ID:   img1ID,
				Size: 100 * 1024 * 1024,
				RootFS: types.RootFS{
					Layers: []string{"sha256:layerA", "sha256:layerB"},
				},
			}, nil, nil
		case img2ID:
			return types.ImageInspect{
				ID:   img2ID,
				Size: 50 * 1024 * 1024,
				RootFS: types.RootFS{
					Layers: []string{"sha256:layerA", "sha256:layerC"},
				},
			}, nil, nil
		case img3ID:
			return types.ImageInspect{
				ID:   img3ID,
				Size: 30 * 1024 * 1024,
				RootFS: types.RootFS{
					Layers: []string{"sha256:layerD"},
				},
			}, nil, nil
		default:
			return types.ImageInspect{ID: imageID}, nil, nil
		}
	}

	// Mock volumes:
	// 1. anonVolName (unmounted, orphaned anonymous)
	// 2. namedVolName (mounted by c1, in-use)
	mock.VolumeListFunc = func(ctx context.Context, filter volume.ListOptions) (volume.ListResponse, error) {
		return volume.ListResponse{
			Volumes: []*volume.Volume{
				{
					Name:   anonVolName,
					Driver: "local",
					UsageData: &volume.UsageData{
						Size: 10 * 1024 * 1024,
					},
				},
				{
					Name:   namedVolName,
					Driver: "local",
					UsageData: &volume.UsageData{
						Size: 50 * 1024 * 1024,
					},
				},
			},
		}, nil
	}

	snapshot, err := InspectEngine(context.Background(), mock)
	if err != nil {
		t.Fatalf("InspectEngine failed: %v", err)
	}

	if len(snapshot.Images) != 3 {
		t.Fatalf("expected 3 images, got %d", len(snapshot.Images))
	}
	if len(snapshot.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(snapshot.Volumes))
	}

	// Check img1: should be in-use
	img1Meta := snapshot.Images[0]
	if !img1Meta.InUse {
		t.Errorf("expected img1 to be in use")
	}
	if img1Meta.IsDangling {
		t.Errorf("img1 should not be dangling")
	}

	// Check img2: should be dangling and not in-use
	img2Meta := snapshot.Images[1]
	if img2Meta.InUse {
		t.Errorf("expected img2 to NOT be in use")
	}
	if !img2Meta.IsDangling {
		t.Errorf("expected img2 to be dangling")
	}

	// Check volumes
	anonVol := snapshot.Volumes[0]
	if !anonVol.IsAnonymous || !anonVol.IsOrphaned {
		t.Errorf("expected anonVol to be anonymous and orphaned, got anon=%v orphan=%v", anonVol.IsAnonymous, anonVol.IsOrphaned)
	}

	namedVol := snapshot.Volumes[1]
	if namedVol.IsAnonymous || namedVol.IsOrphaned || !namedVol.InUse {
		t.Errorf("expected namedVol to be in use, non-anon, non-orphan, got inUse=%v anon=%v orphan=%v", namedVol.InUse, namedVol.IsAnonymous, namedVol.IsOrphaned)
	}
}

func TestInspectEngineErrors(t *testing.T) {
	mock := NewMockDockerClient()

	// Container list error
	mock.ContainerListFunc = func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
		return nil, errors.New("container list failed")
	}
	_, err := InspectEngine(context.Background(), mock)
	if err == nil {
		t.Errorf("expected error on container list failure")
	}

	// Image list error
	mock = NewMockDockerClient()
	mock.ImageListFunc = func(ctx context.Context, options image.ListOptions) ([]image.Summary, error) {
		return nil, errors.New("image list failed")
	}
	_, err = InspectEngine(context.Background(), mock)
	if err == nil {
		t.Errorf("expected error on image list failure")
	}

	// Volume list error
	mock = NewMockDockerClient()
	mock.VolumeListFunc = func(ctx context.Context, filter volume.ListOptions) (volume.ListResponse, error) {
		return volume.ListResponse{}, errors.New("volume list failed")
	}
	_, err = InspectEngine(context.Background(), mock)
	if err == nil {
		t.Errorf("expected error on volume list failure")
	}
}
