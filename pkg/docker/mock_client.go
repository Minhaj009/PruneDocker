package docker

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
)

// MockDockerClient provides an in-memory configurable mock of DockerClient for testing.
type MockDockerClient struct {
	PingFunc               func(ctx context.Context) (types.Ping, error)
	ImageListFunc          func(ctx context.Context, options image.ListOptions) ([]image.Summary, error)
	ImageInspectWithRawFunc func(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
	ImageRemoveFunc        func(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error)
	ContainerListFunc      func(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	VolumeListFunc         func(ctx context.Context, filter volume.ListOptions) (volume.ListResponse, error)
	VolumeRemoveFunc       func(ctx context.Context, volumeID string, force bool) error
	DiskUsageFunc          func(ctx context.Context, options types.DiskUsageOptions) (types.DiskUsage, error)
	CloseFunc              func() error

	// Tracking calls
	RemovedImages  []string
	RemovedVolumes []string
}

var _ DockerClient = (*MockDockerClient)(nil)

func NewMockDockerClient() *MockDockerClient {
	return &MockDockerClient{
		PingFunc: func(ctx context.Context) (types.Ping, error) {
			return types.Ping{APIVersion: "1.45", OSType: "linux"}, nil
		},
		ImageListFunc: func(ctx context.Context, options image.ListOptions) ([]image.Summary, error) {
			return []image.Summary{}, nil
		},
		ImageInspectWithRawFunc: func(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
			return types.ImageInspect{ID: imageID}, nil, nil
		},
		ImageRemoveFunc: func(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error) {
			return []image.DeleteResponse{{Deleted: imageID}}, nil
		},
		ContainerListFunc: func(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
			return []types.Container{}, nil
		},
		VolumeListFunc: func(ctx context.Context, filter volume.ListOptions) (volume.ListResponse, error) {
			return volume.ListResponse{Volumes: []*volume.Volume{}}, nil
		},
		VolumeRemoveFunc: func(ctx context.Context, volumeID string, force bool) error {
			return nil
		},
		DiskUsageFunc: func(ctx context.Context, options types.DiskUsageOptions) (types.DiskUsage, error) {
			return types.DiskUsage{}, nil
		},
		CloseFunc: func() error {
			return nil
		},
	}
}

func (m *MockDockerClient) Ping(ctx context.Context) (types.Ping, error) {
	if m.PingFunc != nil {
		return m.PingFunc(ctx)
	}
	return types.Ping{APIVersion: "1.45"}, nil
}

func (m *MockDockerClient) ImageList(ctx context.Context, options image.ListOptions) ([]image.Summary, error) {
	if m.ImageListFunc != nil {
		return m.ImageListFunc(ctx, options)
	}
	return nil, nil
}

func (m *MockDockerClient) ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
	if m.ImageInspectWithRawFunc != nil {
		return m.ImageInspectWithRawFunc(ctx, imageID)
	}
	return types.ImageInspect{ID: imageID}, nil, nil
}

func (m *MockDockerClient) ImageRemove(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error) {
	m.RemovedImages = append(m.RemovedImages, imageID)
	if m.ImageRemoveFunc != nil {
		return m.ImageRemoveFunc(ctx, imageID, options)
	}
	return []image.DeleteResponse{{Deleted: imageID}}, nil
}

func (m *MockDockerClient) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	if m.ContainerListFunc != nil {
		return m.ContainerListFunc(ctx, options)
	}
	return nil, nil
}

func (m *MockDockerClient) VolumeList(ctx context.Context, filter volume.ListOptions) (volume.ListResponse, error) {
	if m.VolumeListFunc != nil {
		return m.VolumeListFunc(ctx, filter)
	}
	return volume.ListResponse{}, nil
}

func (m *MockDockerClient) VolumeRemove(ctx context.Context, volumeID string, force bool) error {
	m.RemovedVolumes = append(m.RemovedVolumes, volumeID)
	if m.VolumeRemoveFunc != nil {
		return m.VolumeRemoveFunc(ctx, volumeID, force)
	}
	return nil
}

func (m *MockDockerClient) DiskUsage(ctx context.Context, options types.DiskUsageOptions) (types.DiskUsage, error) {
	if m.DiskUsageFunc != nil {
		return m.DiskUsageFunc(ctx, options)
	}
	return types.DiskUsage{}, nil
}

func (m *MockDockerClient) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}
