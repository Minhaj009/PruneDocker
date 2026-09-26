package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/prunedocker/prunedocker/pkg/analyzer"
	"github.com/prunedocker/prunedocker/pkg/cleaner"
	"github.com/prunedocker/prunedocker/pkg/daemon"
	"github.com/prunedocker/prunedocker/pkg/docker"
	"github.com/prunedocker/prunedocker/pkg/ui"
)

// Global CLI Flags
var (
	flagSocket         string
	flagKeepRecent     time.Duration
	flagMinReuse       int
	flagScoreThreshold float64
	flagDryRun         bool
	flagMaxDiskUsage   float64
	flagCheckInterval  time.Duration
	flagMetricsAddr    string
)

func getPolicy() analyzer.ScoringPolicy {
	return analyzer.ScoringPolicy{
		KeepRecent:     flagKeepRecent,
		MinReuse:       flagMinReuse,
		ScoreThreshold: flagScoreThreshold,
	}
}

func getClient(clientOverride docker.DockerClient) (docker.DockerClient, error) {
	if clientOverride != nil {
		return clientOverride, nil
	}
	return docker.NewEngineClient(flagSocket)
}

// NewRootCmd constructs the CLI root command and subcommands.
func NewRootCmd(clientOverride docker.DockerClient) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "prunedocker",
		Short: "PruneDocker: Intelligent Layer-Preserving Docker Cache Optimizer",
		Long: `PruneDocker connects directly to your local Docker socket to analyze image layer ancestry trees,
calculate SHA-256 reuse frequency across multi-stage builds, and safely prune dead intermediate blobs
while strictly preserving warm, high-reuse build caches.`,
	}

	rootCmd.PersistentFlags().StringVar(&flagSocket, "socket", "", "Custom Docker socket path or named pipe (e.g. /var/run/docker.sock or //./pipe/docker_engine)")
	rootCmd.PersistentFlags().DurationVar(&flagKeepRecent, "keep-recent", 72*time.Hour, "Preserve intermediate layers created within this time window")
	rootCmd.PersistentFlags().IntVar(&flagMinReuse, "min-reuse", 2, "Minimum reference count across builds to classify a layer as warm cache")
	rootCmd.PersistentFlags().Float64Var(&flagScoreThreshold, "score-threshold", 0.5, "Minimum cache utility score required to protect an intermediate layer")

	// 1. UI Subcommand
	uiCmd := &cobra.Command{
		Use:   "ui",
		Short: "Launch interactive Terminal TUI dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := getClient(clientOverride)
			if err != nil {
				return err
			}
			defer cli.Close()
			return ui.RunTUI(cli, getPolicy())
		},
	}

	// 2. Prune Subcommand
	pruneCmd := &cobra.Command{
		Use:   "prune",
		Short: "Execute intelligent layer-preserving pruning",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := getClient(clientOverride)
			if err != nil {
				return err
			}
			defer cli.Close()

			ctx := cmd.Context()
			snapshot, err := docker.InspectEngine(ctx, cli)
			if err != nil {
				return fmt.Errorf("failed to inspect engine: %w", err)
			}

			dag := analyzer.BuildDAG(snapshot)
			report := analyzer.EvaluateEngine(dag, snapshot, getPolicy(), time.Now())
			plan := cleaner.BuildPrunePlan(report)

			fmt.Println("================================================================================")
			fmt.Printf(" PRUNEDOCKER OPTIMIZATION PLAN (Dry-Run: %v)\n", flagDryRun)
			fmt.Println("================================================================================")
			fmt.Printf(" Active / Tagged Images:      %d (%.2f MB protected)\n",
				report.ActiveProtectedCount, float64(report.ActiveProtectedBytes)/(1024*1024))
			fmt.Printf(" Warm Build Caches:          %d (%.2f MB strictly preserved)\n",
				report.WarmCacheCount, float64(report.WarmCacheSizeBytes)/(1024*1024))
			fmt.Printf(" Dead Intermediate Leaves:    %d (%.2f MB candidates)\n",
				len(plan.ImageTargets), float64(plan.ProjectedImageBytes)/(1024*1024))
			fmt.Printf(" Orphan Anonymous Volumes:    %d (%.2f MB candidates)\n",
				len(plan.VolumeTargets), float64(plan.ProjectedVolumeBytes)/(1024*1024))
			fmt.Printf(" Total Projected Reclamation: %.2f MB\n",
				float64(plan.ProjectedTotalBytes)/(1024*1024))
			fmt.Println("--------------------------------------------------------------------------------")

			if len(plan.ImageTargets) == 0 && len(plan.VolumeTargets) == 0 {
				fmt.Println("[OK] Storage is already fully optimized. No pruning needed!")
				return nil
			}

			pruner := cleaner.NewPruner(cli)
			execReport, err := pruner.Execute(ctx, plan, flagDryRun)
			if err != nil {
				return fmt.Errorf("prune execution failed: %w", err)
			}

			fmt.Println(" Execution Results:")
			for _, res := range execReport.Results {
				status := "[SUCCESS]"
				if !res.Success {
					status = "[FAILED]"
				}
				fmt.Printf("  %s %-6s %s (%.2f MB) %s\n",
					status, res.Type, docker.CleanID(res.ID),
					float64(res.FreedBytes)/(1024*1024), res.SkippedNote)
			}
			fmt.Println("--------------------------------------------------------------------------------")
			fmt.Printf(" Pruned: %d images, %d volumes. Total Space Reclaimed: %.2f MB in %v\n",
				execReport.ImagesPruned, execReport.VolumesPruned,
				float64(execReport.ReclaimedBytes)/(1024*1024), execReport.Duration)
			fmt.Println("================================================================================")
			return nil
		},
	}
	pruneCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Simulate pruning without deleting any images or volumes")

	// 3. Analyze Subcommand
	analyzeCmd := &cobra.Command{
		Use:   "analyze",
		Short: "Inspect Docker layers and display cache reuse DAG without making changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := getClient(clientOverride)
			if err != nil {
				return err
			}
			defer cli.Close()

			ctx := cmd.Context()
			snapshot, err := docker.InspectEngine(ctx, cli)
			if err != nil {
				return fmt.Errorf("failed to inspect engine: %w", err)
			}

			dag := analyzer.BuildDAG(snapshot)
			report := analyzer.EvaluateEngine(dag, snapshot, getPolicy(), time.Now())

			fmt.Println("================================================================================")
			fmt.Println(" PRUNEDOCKER LAYER DEPENDENCY & CACHE UTILITY REPORT")
			fmt.Println("================================================================================")
			fmt.Printf(" Total Images: %d | Total Layers: %d | Active Containers: %d\n\n",
				dag.TotalImages, dag.TotalLayers, snapshot.ActiveContainers)

			fmt.Printf(" %-12s %-24s %-10s %-7s %-8s %-18s\n",
				"IMAGE ID", "TAG", "SIZE", "REUSE", "SCORE", "STATUS")
			fmt.Println(" " + strings.Repeat("-", 82))

			for _, img := range report.Images {
				tag := "<none>"
				if len(img.Metadata.RepoTags) > 0 && img.Metadata.RepoTags[0] != "<none>:<none>" {
					tag = img.Metadata.RepoTags[0]
					if len(tag) > 23 {
						tag = tag[:20] + "..."
					}
				}

				scoreStr := fmt.Sprintf("%.2f", img.Score)
				if img.Score > 999 {
					scoreStr = "INF"
				}

				fmt.Printf(" %-12s %-24s %-10s %-7d %-8s %-18s\n",
					img.Metadata.ShortID, tag, fmt.Sprintf("%.1f MB", img.Metadata.SizeMB),
					img.EffectiveReuse, scoreStr, img.Category)
			}

			fmt.Println("\n Volumes:")
			fmt.Println(" " + strings.Repeat("-", 82))
			for _, vol := range report.Volumes {
				fmt.Printf(" %-40s %-12s %-18s\n",
					docker.CleanID(vol.Metadata.Name),
					fmt.Sprintf("%.1f MB", float64(vol.Metadata.SizeBytes)/(1024*1024)),
					vol.Category)
			}

			plan := cleaner.BuildPrunePlan(report)
			fmt.Println("\n Summary:")
			fmt.Println(" " + strings.Repeat("-", 82))
			fmt.Printf("  Protected Active:      %d images (%.2f MB)\n",
				report.ActiveProtectedCount, float64(report.ActiveProtectedBytes)/(1024*1024))
			fmt.Printf("  Warm Cache Kept:       %d images (%.2f MB)\n",
				report.WarmCacheCount, float64(report.WarmCacheSizeBytes)/(1024*1024))
			fmt.Printf("  Reclaimable Dead:      %d images, %d volumes (%.2f MB)\n",
				len(plan.ImageTargets), len(plan.VolumeTargets),
				float64(plan.ProjectedTotalBytes)/(1024*1024))
			fmt.Println("================================================================================")
			return nil
		},
	}

	// 4. Daemon Subcommand
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start as low-overhead background daemon with Prometheus metrics exporter",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli, err := getClient(clientOverride)
			if err != nil {
				return err
			}
			defer cli.Close()

			cfg := daemon.Config{
				MaxDiskUsagePercent: flagMaxDiskUsage,
				CheckInterval:       flagCheckInterval,
				MetricsAddr:         flagMetricsAddr,
				DryRun:              flagDryRun,
				Policy:              getPolicy(),
			}

			d := daemon.NewDaemon(cli, cfg)

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-sigChan
				fmt.Println("\nReceived shutdown signal. Stopping PruneDocker daemon...")
				cancel()
			}()

			return d.Start(ctx)
		},
	}
	daemonCmd.Flags().Float64Var(&flagMaxDiskUsage, "max-disk-usage", 80.0, "Prune only when disk usage ratio exceeds this percentage (e.g. 80)")
	daemonCmd.Flags().DurationVar(&flagCheckInterval, "check-interval", 30*time.Minute, "Periodic check interval")
	daemonCmd.Flags().StringVar(&flagMetricsAddr, "metrics-addr", ":9199", "Prometheus HTTP metrics exporter bind address")
	daemonCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Run daemon in simulation mode without deleting items")

	rootCmd.AddCommand(uiCmd, pruneCmd, analyzeCmd, daemonCmd)
	return rootCmd
}

func main() {
	rootCmd := NewRootCmd(nil)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
