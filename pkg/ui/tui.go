package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/prunedocker/prunedocker/pkg/analyzer"
	"github.com/prunedocker/prunedocker/pkg/cleaner"
	"github.com/prunedocker/prunedocker/pkg/docker"
)

// UI Color Palette & Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#5A56E0")).
			Padding(0, 1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true)

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#2E5BFF")).
			Padding(0, 2)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#777777")).
				Background(lipgloss.Color("#222222")).
				Padding(0, 2)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#444444")).
			Padding(0, 1)

	badgeActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00E5FF")).
			SetString("[ACTIVE IN USE]")

	badgeWarm = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00E676")).
			SetString("[WARM CACHE KEPT]")

	badgePrune = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF1744")).
			SetString("[DEAD LEAF PRUNE]")

	badgeOrphan = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFD600")).
			SetString("[ORPHAN VOL PRUNE]")

	badgeSavedVol = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#82B1FF")).
			SetString("[SAVED VOLUME]")

	helpKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00E5FF"))

	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))
)

// Model represents the state of the Bubbletea TUI application.
type Model struct {
	Client          docker.DockerClient
	Policy          analyzer.ScoringPolicy
	Snapshot        *docker.EngineSnapshot
	DAG             *analyzer.DependencyDAG
	Report          *analyzer.AnalysisReport
	Pruner          *cleaner.Pruner
	LastPruneReport *cleaner.PruneReport
	ActiveTab       int // 0: Images, 1: Volumes, 2: Prune Plan
	Cursor          int
	DryRun          bool
	ConfirmingPrune bool
	StatusMessage   string
	Width           int
	Height          int
	Err             error
}

// RescanMsg triggers a re-inspection of the Docker daemon.
type RescanMsg struct {
	Snapshot *docker.EngineSnapshot
	DAG      *analyzer.DependencyDAG
	Report   *analyzer.AnalysisReport
	Err      error
}

// PruneCompleteMsg signals that a pruning execution finished.
type PruneCompleteMsg struct {
	Report *cleaner.PruneReport
	Err    error
}

// NewModel initializes the TUI Model.
func NewModel(client docker.DockerClient, policy analyzer.ScoringPolicy) Model {
	m := Model{
		Client:        client,
		Policy:        policy,
		Pruner:        cleaner.NewPruner(client),
		ActiveTab:     0,
		Cursor:        0,
		DryRun:        true, // default to safe dry-run mode
		StatusMessage: "Ready. Press [p] to prune, [d] to toggle dry-run, [tab] to switch tabs.",
	}
	m.RefreshSync()
	return m
}

// RefreshSync executes a synchronous scan of the engine.
func (m *Model) RefreshSync() {
	if m.Client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snapshot, err := docker.InspectEngine(ctx, m.Client)
	if err != nil {
		m.Err = err
		m.StatusMessage = fmt.Sprintf("Scan error: %v", err)
		return
	}

	dag := analyzer.BuildDAG(snapshot)
	report := analyzer.EvaluateEngine(dag, snapshot, m.Policy, time.Now())

	m.Snapshot = snapshot
	m.DAG = dag
	m.Report = report
	m.Err = nil
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil

	case RescanMsg:
		if msg.Err != nil {
			m.Err = msg.Err
			m.StatusMessage = fmt.Sprintf("Rescan error: %v", msg.Err)
		} else {
			m.Snapshot = msg.Snapshot
			m.DAG = msg.DAG
			m.Report = msg.Report
			m.Err = nil
			m.Cursor = 0
			m.StatusMessage = "Scan refreshed successfully."
		}
		return m, nil

	case PruneCompleteMsg:
		if msg.Err != nil {
			m.Err = msg.Err
			m.StatusMessage = fmt.Sprintf("Prune failed: %v", msg.Err)
		} else {
			m.LastPruneReport = msg.Report
			if msg.Report.DryRun {
				m.StatusMessage = fmt.Sprintf("Dry-run complete! Reclaimable: %.2f MB across %d items.",
					float64(msg.Report.ReclaimedBytes)/(1024*1024),
					msg.Report.ImagesPruned+msg.Report.VolumesPruned)
			} else {
				m.StatusMessage = fmt.Sprintf("Live prune complete! Successfully freed %.2f MB.",
					float64(msg.Report.ReclaimedBytes)/(1024*1024))
			}
			m.RefreshSync()
		}
		m.ConfirmingPrune = false
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "tab":
			m.ActiveTab = (m.ActiveTab + 1) % 3
			m.Cursor = 0
			return m, nil

		case "up", "k":
			if m.Cursor > 0 {
				m.Cursor--
			}
			return m, nil

		case "down", "j":
			maxItems := 0
			if m.Report != nil {
				if m.ActiveTab == 0 {
					maxItems = len(m.Report.Images)
				} else if m.ActiveTab == 1 {
					maxItems = len(m.Report.Volumes)
				}
			}
			if m.Cursor < maxItems-1 {
				m.Cursor++
			}
			return m, nil

		case "d":
			m.DryRun = !m.DryRun
			if m.DryRun {
				m.StatusMessage = "Mode changed: DRY-RUN ENABLED (Safe, zero deletions)"
			} else {
				m.StatusMessage = "Mode changed: LIVE PRUNING ENABLED (Real deletions will occur!)"
			}
			return m, nil

		case "r":
			m.StatusMessage = "Rescanning Docker Engine..."
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				snap, err := docker.InspectEngine(ctx, m.Client)
				if err != nil {
					return RescanMsg{Err: err}
				}
				dag := analyzer.BuildDAG(snap)
				rep := analyzer.EvaluateEngine(dag, snap, m.Policy, time.Now())
				return RescanMsg{Snapshot: snap, DAG: dag, Report: rep}
			}

		case "p":
			if m.Report == nil || (m.Report.PruneCandidateCount == 0 && m.Report.OrphanVolumeCount == 0) {
				m.StatusMessage = "No prune candidates detected. Storage is already optimized!"
				return m, nil
			}
			m.ConfirmingPrune = true
			if m.DryRun {
				m.StatusMessage = "Confirm DRY-RUN simulation? Press [y] to confirm, [n] to cancel."
			} else {
				m.StatusMessage = "WARNING: LIVE PRUNE! Permanently delete candidates? Press [y] to confirm, [n] to cancel."
			}
			return m, nil

		case "y":
			if m.ConfirmingPrune && m.Report != nil {
				m.StatusMessage = "Executing pruning plan..."
				plan := cleaner.BuildPrunePlan(m.Report)
				dryRun := m.DryRun
				pruner := m.Pruner
				return m, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					rep, err := pruner.Execute(ctx, plan, dryRun)
					return PruneCompleteMsg{Report: rep, Err: err}
				}
			}

		case "n", "esc":
			if m.ConfirmingPrune {
				m.ConfirmingPrune = false
				m.StatusMessage = "Prune operation cancelled."
				return m, nil
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	var b strings.Builder

	// 1. Header
	modeBadge := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00E676")).Render("[DRY-RUN ON]")
	if !m.DryRun {
		modeBadge = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF1744")).Render("[LIVE MODE]")
	}

	header := fmt.Sprintf("%s  %s  %s\n",
		titleStyle.Render(" PRUNEDOCKER "),
		subtitleStyle.Render("Intelligent Layer-Preserving Cache Optimizer"),
		modeBadge,
	)
	b.WriteString(header)

	// 2. Summary Card
	if m.Report != nil {
		warmMB := float64(m.Report.WarmCacheSizeBytes) / (1024 * 1024)
		activeMB := float64(m.Report.ActiveProtectedBytes) / (1024 * 1024)
		reclaimMB := float64(m.Report.PruneCandidateSizeBytes+m.Report.OrphanVolumeSizeBytes) / (1024 * 1024)

		summaryContent := fmt.Sprintf(
			"Protected: %s (%d img, %.1f MB) | %s (%d img, %.1f MB)\n"+
				"Candidates: %s (%d dead img) | %s (%d orphan vol) | Reclaimable: %.1f MB",
			badgeActive.String(), m.Report.ActiveProtectedCount, activeMB,
			badgeWarm.String(), m.Report.WarmCacheCount, warmMB,
			badgePrune.String(), m.Report.PruneCandidateCount,
			badgeOrphan.String(), m.Report.OrphanVolumeCount,
			reclaimMB,
		)
		b.WriteString(cardStyle.Render(summaryContent) + "\n\n")
	}

	// 3. Navigation Tabs
	tab0 := tabInactiveStyle.Render("1. Images & Layers")
	tab1 := tabInactiveStyle.Render("2. Volumes")
	tab2 := tabInactiveStyle.Render("3. Prune Plan")

	switch m.ActiveTab {
	case 0:
		tab0 = tabActiveStyle.Render("1. Images & Layers")
	case 1:
		tab1 = tabActiveStyle.Render("2. Volumes")
	case 2:
		tab2 = tabActiveStyle.Render("3. Prune Plan")
	}
	b.WriteString(fmt.Sprintf("%s %s %s\n\n", tab0, tab1, tab2))

	// 4. Tab Content
	if m.Report == nil {
		b.WriteString("  No engine snapshot available.\n")
	} else {
		switch m.ActiveTab {
		case 0:
			b.WriteString(m.renderImagesTab())
		case 1:
			b.WriteString(m.renderVolumesTab())
		case 2:
			b.WriteString(m.renderPlanTab())
		}
	}

	// 5. Status & Footer Help Bar
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD600")).Render("Status: "+m.StatusMessage) + "\n")
	helpBar := fmt.Sprintf("%s %s  %s %s  %s %s  %s %s  %s %s  %s %s",
		helpKeyStyle.Render("[Tab]"), helpDescStyle.Render("Switch View"),
		helpKeyStyle.Render("[p]"), helpDescStyle.Render("Prune"),
		helpKeyStyle.Render("[d]"), helpDescStyle.Render("Toggle Dry-Run"),
		helpKeyStyle.Render("[r]"), helpDescStyle.Render("Rescan"),
		helpKeyStyle.Render("[j/k]"), helpDescStyle.Render("Scroll"),
		helpKeyStyle.Render("[q]"), helpDescStyle.Render("Quit"),
	)
	b.WriteString(helpBar + "\n")

	return b.String()
}

func (m Model) renderImagesTab() string {
	if len(m.Report.Images) == 0 {
		return "  No images found in local Docker engine.\n"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  %-14s %-25s %-10s %-8s %-8s %-20s\n",
		"IMAGE ID", "TAG / REPO", "SIZE", "REUSE", "SCORE", "STATUS"))
	b.WriteString(strings.Repeat("-", 90) + "\n")

	for i, img := range m.Report.Images {
		cursor := "  "
		if i == m.Cursor {
			cursor = "> "
		}

		tagStr := "<none>"
		if len(img.Metadata.RepoTags) > 0 && img.Metadata.RepoTags[0] != "<none>:<none>" {
			tagStr = img.Metadata.RepoTags[0]
			if len(tagStr) > 24 {
				tagStr = tagStr[:21] + "..."
			}
		}

		statusBadge := badgeActive.String()
		switch img.Category {
		case analyzer.CategoryWarmCache:
			statusBadge = badgeWarm.String()
		case analyzer.CategoryPruneCandidate:
			statusBadge = badgePrune.String()
		}

		scoreStr := fmt.Sprintf("%.2f", img.Score)
		if img.Score > 999 {
			scoreStr = "INF"
		}

		line := fmt.Sprintf("%s%-12s %-25s %-10s %-8d %-8s %s\n",
			cursor,
			img.Metadata.ShortID,
			tagStr,
			fmt.Sprintf("%.1f MB", img.Metadata.SizeMB),
			img.EffectiveReuse,
			scoreStr,
			statusBadge,
		)
		b.WriteString(line)
	}

	return b.String()
}

func (m Model) renderVolumesTab() string {
	if len(m.Report.Volumes) == 0 {
		return "  No volumes found in local Docker engine.\n"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  %-40s %-12s %-22s\n", "VOLUME NAME", "SIZE", "STATUS"))
	b.WriteString(strings.Repeat("-", 80) + "\n")

	for i, vol := range m.Report.Volumes {
		cursor := "  "
		if i == m.Cursor {
			cursor = "> "
		}

		nameStr := vol.Metadata.Name
		if len(nameStr) > 38 {
			nameStr = nameStr[:35] + "..."
		}

		statusBadge := badgeSavedVol.String()
		if vol.Category == analyzer.CategoryOrphanVolume {
			statusBadge = badgeOrphan.String()
		}

		line := fmt.Sprintf("%s%-40s %-12s %s\n",
			cursor,
			nameStr,
			fmt.Sprintf("%.1f MB", float64(vol.Metadata.SizeBytes)/(1024*1024)),
			statusBadge,
		)
		b.WriteString(line)
	}

	return b.String()
}

func (m Model) renderPlanTab() string {
	plan := cleaner.BuildPrunePlan(m.Report)
	var b strings.Builder

	b.WriteString(fmt.Sprintf("  Targets to Prune: %d Dead Images, %d Orphan Volumes\n",
		len(plan.ImageTargets), len(plan.VolumeTargets)))
	b.WriteString(fmt.Sprintf("  Total Projected Disk Reclamation: %.2f MB\n",
		float64(plan.ProjectedTotalBytes)/(1024*1024)))
	b.WriteString(fmt.Sprintf("  Warm Build Cache Strictly Preserved: %.2f MB (%d images)\n",
		float64(plan.WarmCacheProtectedByt)/(1024*1024), plan.WarmCacheProtectedCnt))
	b.WriteString(strings.Repeat("-", 80) + "\n")

	if len(plan.ImageTargets) == 0 && len(plan.VolumeTargets) == 0 {
		b.WriteString("  [OK] No cleanup needed! All local layers are active or valuable warm cache.\n")
		return b.String()
	}

	for _, img := range plan.ImageTargets {
		b.WriteString(fmt.Sprintf("  - [IMAGE]  %s (%.1f MB) - %s\n",
			img.Metadata.ShortID, img.Metadata.SizeMB, img.Reason))
	}
	for _, vol := range plan.VolumeTargets {
		b.WriteString(fmt.Sprintf("  - [VOLUME] %s (%.1f MB) - %s\n",
			docker.CleanID(vol.Metadata.Name),
			float64(vol.Metadata.SizeBytes)/(1024*1024),
			vol.Reason))
	}

	return b.String()
}

// RunTUI launches the interactive Bubbletea interface.
func RunTUI(client docker.DockerClient, policy analyzer.ScoringPolicy) error {
	m := NewModel(client, policy)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
