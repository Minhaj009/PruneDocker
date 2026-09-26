package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/prunedocker/prunedocker/pkg/docker"
)

func createCLITestMock() *docker.MockDockerClient {
	mock := docker.NewMockDockerClient()
	now := time.Now()

	mock.ImageListFunc = func(ctx context.Context, options image.ListOptions) ([]image.Summary, error) {
		return []image.Summary{
			{
				ID:       "dead_img_cli_12345678",
				RepoTags: []string{"<none>:<none>"},
				Size:     50 * 1024 * 1024,
				Created:  now.Add(-200 * time.Hour).Unix(),
			},
			{
				ID:       "prod_img_cli_12345678",
				RepoTags: []string{"web:latest"},
				Size:     100 * 1024 * 1024,
				Created:  now.Add(-200 * time.Hour).Unix(),
			},
		}, nil
	}

	mock.VolumeListFunc = func(ctx context.Context, filter volume.ListOptions) (volume.ListResponse, error) {
		return volume.ListResponse{
			Volumes: []*volume.Volume{
				{
					Name:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					Driver: "local",
					UsageData: &volume.UsageData{
						Size: 10 * 1024 * 1024,
					},
				},
			},
		}, nil
	}

	return mock
}

func TestCLIHelp(t *testing.T) {
	cmd := NewRootCmd(nil)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error running --help: %v", err)
	}

	out := buf.String()
	expectedSubcommands := []string{"ui", "prune", "analyze", "daemon"}
	for _, sub := range expectedSubcommands {
		if !strings.Contains(out, sub) {
			t.Errorf("expected help output to contain subcommand %q", sub)
		}
	}
}

func TestCLIAnalyze(t *testing.T) {
	mock := createCLITestMock()
	cmd := NewRootCmd(mock)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"analyze"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error running analyze: %v", err)
	}

	// Analyze should output table and summary
	if len(mock.RemovedImages) != 0 || len(mock.RemovedVolumes) != 0 {
		t.Errorf("analyze must never delete items!")
	}
}

func TestCLIPruneDryRun(t *testing.T) {
	mock := createCLITestMock()
	cmd := NewRootCmd(mock)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"prune", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error running prune --dry-run: %v", err)
	}

	// Verify no actual deletions occurred
	if len(mock.RemovedImages) != 0 {
		t.Errorf("expected 0 removed images in dry-run, got %d", len(mock.RemovedImages))
	}
	if len(mock.RemovedVolumes) != 0 {
		t.Errorf("expected 0 removed volumes in dry-run, got %d", len(mock.RemovedVolumes))
	}
}

func TestCLIPruneLive(t *testing.T) {
	mock := createCLITestMock()
	cmd := NewRootCmd(mock)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"prune"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error running prune live: %v", err)
	}

	// Verify dead image and orphan volume were removed!
	if len(mock.RemovedImages) != 1 {
		t.Errorf("expected 1 removed image, got %d", len(mock.RemovedImages))
	}
	if len(mock.RemovedVolumes) != 1 {
		t.Errorf("expected 1 removed volume, got %d", len(mock.RemovedVolumes))
	}
}
