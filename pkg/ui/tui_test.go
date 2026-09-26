package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/prunedocker/prunedocker/pkg/analyzer"
	"github.com/prunedocker/prunedocker/pkg/cleaner"
	"github.com/prunedocker/prunedocker/pkg/docker"
)

func createTestModel() (Model, *docker.MockDockerClient) {
	mock := docker.NewMockDockerClient()
	now := time.Now()

	mock.ImageListFunc = func(ctx context.Context, options image.ListOptions) ([]image.Summary, error) {
		return []image.Summary{
			{
				ID:       "dead_img_12345678",
				RepoTags: []string{"<none>:<none>"},
				Size:     100 * 1024 * 1024,
				Created:  now.Add(-200 * time.Hour).Unix(),
			},
			{
				ID:       "warm_img_12345678",
				RepoTags: []string{"<none>:<none>"},
				Size:     50 * 1024 * 1024,
				Created:  now.Add(-5 * time.Hour).Unix(),
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
						Size: 20 * 1024 * 1024,
					},
				},
			},
		}, nil
	}

	m := NewModel(mock, analyzer.DefaultPolicy())
	return m, mock
}

func TestTUIModelInit(t *testing.T) {
	m, _ := createTestModel()

	if !m.DryRun {
		t.Errorf("expected default DryRun = true")
	}
	if m.ActiveTab != 0 {
		t.Errorf("expected initial ActiveTab = 0, got %d", m.ActiveTab)
	}
	if m.Report == nil {
		t.Fatalf("expected non-nil Report")
	}

	view := m.View()
	if !strings.Contains(view, "PRUNEDOCKER") {
		t.Errorf("expected view to contain 'PRUNEDOCKER'")
	}
	if !strings.Contains(view, "DRY-RUN ON") {
		t.Errorf("expected view to indicate dry run is active")
	}
}

func TestTUIKeyHandling(t *testing.T) {
	m, _ := createTestModel()

	// 1. Tab switching
	mod, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mod.(Model)
	if m.ActiveTab != 1 {
		t.Errorf("expected ActiveTab=1 after tab key, got %d", m.ActiveTab)
	}

	mod, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = mod.(Model)
	if m.ActiveTab != 2 {
		t.Errorf("expected ActiveTab=2, got %d", m.ActiveTab)
	}

	// 2. Toggle dry-run
	mod, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = mod.(Model)
	if m.DryRun {
		t.Errorf("expected DryRun=false after pressing 'd'")
	}

	// 3. Arrow navigation
	mod, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = mod.(Model)
	mod, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = mod.(Model)
	if m.Cursor != 0 {
		t.Errorf("expected cursor at 0 after up key, got %d", m.Cursor)
	}

	// 4. Prune confirmation and cancel
	mod, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = mod.(Model)
	if !m.ConfirmingPrune {
		t.Errorf("expected ConfirmingPrune=true after pressing 'p'")
	}

	mod, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = mod.(Model)
	if m.ConfirmingPrune {
		t.Errorf("expected ConfirmingPrune=false after pressing 'n'")
	}

	// 5. Confirming prune with 'y' produces a command
	mod, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m = mod.(Model)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Errorf("expected non-nil tea.Cmd upon confirming prune with 'y'")
	}

	// 6. Quit key
	_, quitCmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quitCmd == nil {
		t.Errorf("expected non-nil quit command")
	}
}

func TestTUIPruneCompleteMsg(t *testing.T) {
	m, _ := createTestModel()

	report := &cleaner.PruneReport{
		DryRun:         true,
		ReclaimedBytes: 100 * 1024 * 1024,
		ImagesPruned:   1,
		VolumesPruned:  1,
	}

	mod, _ := m.Update(PruneCompleteMsg{Report: report})
	m = mod.(Model)

	if m.LastPruneReport == nil {
		t.Fatalf("expected LastPruneReport to be recorded")
	}
	if !strings.Contains(m.StatusMessage, "Dry-run complete") {
		t.Errorf("expected status message to reflect dry-run completion, got: %s", m.StatusMessage)
	}
}
