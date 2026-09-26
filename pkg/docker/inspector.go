package docker

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
)

var hex64Regex = regexp.MustCompile("^[a-f0-9]{64}$")

// IsAnonymousVolume checks if a volume name matches standard Docker 64-char hex format.
func IsAnonymousVolume(name string) bool {
	return hex64Regex.MatchString(strings.ToLower(name))
}

// CleanID formats a Docker SHA-256 ID into a clean 12-char hex string.
func CleanID(id string) string {
	cleaned := strings.TrimPrefix(id, "sha256:")
	if len(cleaned) > 12 {
		return cleaned[:12]
	}
	return cleaned
}

// ImageMetadata contains parsed and categorized details for a Docker image.
type ImageMetadata struct {
	ID         string    `json:"id"`
	ShortID    string    `json:"short_id"`
	ParentID   string    `json:"parent_id"`
	RepoTags   []string  `json:"repo_tags"`
	Layers     []string  `json:"layers"`
	LayerCount int       `json:"layer_count"`
	SizeBytes  int64     `json:"size_bytes"`
	SizeMB     float64   `json:"size_mb"`
	CreatedAt  time.Time `json:"created_at"`
	InUse      bool      `json:"in_use"`
	IsDangling bool      `json:"is_dangling"`
}

// VolumeMetadata contains parsed details for a Docker volume.
type VolumeMetadata struct {
	Name        string `json:"name"`
	Driver      string `json:"driver"`
	Mountpoint  string `json:"mountpoint"`
	SizeBytes   int64  `json:"size_bytes"`
	InUse       bool   `json:"in_use"`
	IsAnonymous bool   `json:"is_anonymous"`
	IsOrphaned  bool   `json:"is_orphaned"`
}

// EngineSnapshot encapsulates the complete state retrieved from the Docker Engine.
type EngineSnapshot struct {
	Images               []ImageMetadata  `json:"images"`
	Volumes              []VolumeMetadata `json:"volumes"`
	ActiveContainers     int              `json:"active_containers"`
	TotalImageSizeBytes  int64            `json:"total_image_size_bytes"`
	TotalVolumeSizeBytes int64            `json:"total_volume_size_bytes"`
}

// InspectEngine inspects the local Docker daemon to produce a comprehensive EngineSnapshot.
func InspectEngine(ctx context.Context, client DockerClient) (*EngineSnapshot, error) {
	// 1. Gather container data to pin in-use images and mounted volumes
	containers, err := client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	activeImageIDs := make(map[string]bool)
	mountedVolumeNames := make(map[string]bool)

	for _, c := range containers {
		activeImageIDs[c.ImageID] = true
		activeImageIDs[c.Image] = true
		for _, m := range c.Mounts {
			if m.Name != "" {
				mountedVolumeNames[m.Name] = true
			}
		}
	}

	// 2. Gather image metadata
	images, err := client.ImageList(ctx, image.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	imageMetas := make([]ImageMetadata, 0, len(images))
	var totalImageSize int64

	for _, img := range images {
		inspect, _, err := client.ImageInspectWithRaw(ctx, img.ID)
		var layers []string
		var parentID string
		var createdTime time.Time
		size := img.Size

		if err == nil {
			layers = inspect.RootFS.Layers
			parentID = inspect.Parent
			if inspect.Size > 0 {
				size = inspect.Size
			}
			if inspect.Created != "" {
				if parsed, parseErr := time.Parse(time.RFC3339Nano, inspect.Created); parseErr == nil {
					createdTime = parsed
				} else if parsed, parseErr = time.Parse(time.RFC3339, inspect.Created); parseErr == nil {
					createdTime = parsed
				}
			}
		}

		if createdTime.IsZero() && img.Created > 0 {
			createdTime = time.Unix(img.Created, 0)
		}

		isDangling := len(img.RepoTags) == 0 || (len(img.RepoTags) == 1 && img.RepoTags[0] == "<none>:<none>")
		inUse := activeImageIDs[img.ID] || activeImageIDs[CleanID(img.ID)]
		for _, tag := range img.RepoTags {
			if activeImageIDs[tag] {
				inUse = true
				break
			}
		}

		sizeMB := float64(size) / (1024 * 1024)
		totalImageSize += size

		imageMetas = append(imageMetas, ImageMetadata{
			ID:         img.ID,
			ShortID:    CleanID(img.ID),
			ParentID:   parentID,
			RepoTags:   img.RepoTags,
			Layers:     layers,
			LayerCount: len(layers),
			SizeBytes:  size,
			SizeMB:     sizeMB,
			CreatedAt:  createdTime,
			InUse:      inUse,
			IsDangling: isDangling,
		})
	}

	// 3. Gather volume metadata
	volumeResp, err := client.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}

	volumeMetas := make([]VolumeMetadata, 0, len(volumeResp.Volumes))
	var totalVolSize int64

	for _, v := range volumeResp.Volumes {
		inUse := mountedVolumeNames[v.Name]
		isAnon := IsAnonymousVolume(v.Name)
		isOrphaned := !inUse && isAnon

		var sizeBytes int64
		if v.UsageData != nil {
			sizeBytes = v.UsageData.Size
		}
		totalVolSize += sizeBytes

		volumeMetas = append(volumeMetas, VolumeMetadata{
			Name:        v.Name,
			Driver:      v.Driver,
			Mountpoint:  v.Mountpoint,
			SizeBytes:   sizeBytes,
			InUse:       inUse,
			IsAnonymous: isAnon,
			IsOrphaned:  isOrphaned,
		})
	}

	return &EngineSnapshot{
		Images:               imageMetas,
		Volumes:              volumeMetas,
		ActiveContainers:     len(containers),
		TotalImageSizeBytes:  totalImageSize,
		TotalVolumeSizeBytes: totalVolSize,
	}, nil
}
