package docker

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

// DockerClient defines the interface required by PruneDocker to communicate with the Docker daemon.
type DockerClient interface {
	Ping(ctx context.Context) (types.Ping, error)
	ImageList(ctx context.Context, options image.ListOptions) ([]image.Summary, error)
	ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
	ImageRemove(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error)
	ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	VolumeList(ctx context.Context, filter volume.ListOptions) (volume.ListResponse, error)
	VolumeRemove(ctx context.Context, volumeID string, force bool) error
	DiskUsage(ctx context.Context, options types.DiskUsageOptions) (types.DiskUsage, error)
	Close() error
}

// EngineClient wraps the official Docker SDK client.Client.
type EngineClient struct {
	cli *client.Client
}

// Verify that EngineClient implements DockerClient.
var _ DockerClient = (*EngineClient)(nil)

// ResolveDefaultHost determines the appropriate Docker daemon endpoint based on the operating system.
func ResolveDefaultHost(customHost string) string {
	if customHost != "" {
		return customHost
	}

	if envHost := os.Getenv("DOCKER_HOST"); envHost != "" {
		return envHost
	}

	if runtime.GOOS == "windows" {
		// Prefer standard Windows named pipe; docker-desktop engine pipe fallback
		return "npipe:////./pipe/docker_engine"
	}
	return "unix:///var/run/docker.sock"
}

// NewEngineClient creates a new Docker client connected to the local socket or custom host.
func NewEngineClient(customHost string) (*EngineClient, error) {
	host := ResolveDefaultHost(customHost)

	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}

	if host != "" {
		// If scheme is missing on Unix path, prepend unix://
		if runtime.GOOS != "windows" && !strings.Contains(host, "://") && strings.HasPrefix(host, "/") {
			opts = append(opts, client.WithHost("unix://"+host))
		} else {
			opts = append(opts, client.WithHost(host))
		}
	} else {
		opts = append(opts, client.FromEnv)
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &EngineClient{cli: cli}, nil
}

// Ping checks if the Docker daemon is reachable.
func (c *EngineClient) Ping(ctx context.Context) (types.Ping, error) {
	return c.cli.Ping(ctx)
}

// ImageList returns list of images.
func (c *EngineClient) ImageList(ctx context.Context, options image.ListOptions) ([]image.Summary, error) {
	return c.cli.ImageList(ctx, options)
}

// ImageInspectWithRaw returns image inspection data.
func (c *EngineClient) ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error) {
	return c.cli.ImageInspectWithRaw(ctx, imageID)
}

// ImageRemove deletes an image.
func (c *EngineClient) ImageRemove(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error) {
	return c.cli.ImageRemove(ctx, imageID, options)
}

// ContainerList lists containers.
func (c *EngineClient) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	return c.cli.ContainerList(ctx, options)
}

// VolumeList lists volumes.
func (c *EngineClient) VolumeList(ctx context.Context, filter volume.ListOptions) (volume.ListResponse, error) {
	return c.cli.VolumeList(ctx, filter)
}

// VolumeRemove deletes a volume.
func (c *EngineClient) VolumeRemove(ctx context.Context, volumeID string, force bool) error {
	return c.cli.VolumeRemove(ctx, volumeID, force)
}

// DiskUsage returns storage usage statistics.
func (c *EngineClient) DiskUsage(ctx context.Context, options types.DiskUsageOptions) (types.DiskUsage, error) {
	return c.cli.DiskUsage(ctx, options)
}

// Close closes the underlying client connection.
func (c *EngineClient) Close() error {
	return c.cli.Close()
}
