package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/ineslino/azpipe/internal/analysis"
	"github.com/ineslino/azpipe/internal/ui"
)

var pipelinesCmd = &cobra.Command{
	Use:   "pipelines",
	Short: "Inspect Azure DevOps pipelines",
}

var pipelinesListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all pipelines in a project",
	Example: "  azpipe pipelines list --org myorg --project myproject",
	RunE:    runPipelinesList,
}

var pipelinesRunsCmd = &cobra.Command{
	Use:     "runs <pipeline-id>",
	Short:   "Show the last N runs for a pipeline",
	Args:    cobra.ExactArgs(1),
	Example: "  azpipe pipelines runs 42 --org myorg --project myproject -n 10",
	RunE:    runPipelinesRuns,
}

var pipelinesAnalyzeCmd = &cobra.Command{
	Use:   "analyze <pipeline-id>",
	Short: "Analyze pipeline health: avg duration, failure rate, flaky stages",
	Args:  cobra.ExactArgs(1),
	Example: `  azpipe pipelines analyze 42 --org myorg --project myproject
  azpipe pipelines analyze 42 -n 50 --output json`,
	RunE: runPipelinesAnalyze,
}

var pipelinesWatchCmd = &cobra.Command{
	Use:     "watch <pipeline-id>",
	Short:   "Live-poll the active run of a pipeline (bubbletea TUI)",
	Args:    cobra.ExactArgs(1),
	Example: "  azpipe pipelines watch 42 --org myorg --project myproject",
	RunE:    runPipelinesWatch,
}

var (
	runsLimit     int
	analyzeLimit  int
	watchInterval int
)

func init() {
	pipelinesRunsCmd.Flags().IntVarP(&runsLimit, "limit", "n", 20, "Number of recent runs to show")
	pipelinesAnalyzeCmd.Flags().IntVarP(&analyzeLimit, "limit", "n", 25, "Number of recent runs to analyse")
	pipelinesWatchCmd.Flags().IntVar(&watchInterval, "interval", 5, "Poll interval in seconds")

	pipelinesCmd.AddCommand(pipelinesListCmd)
	pipelinesCmd.AddCommand(pipelinesRunsCmd)
	pipelinesCmd.AddCommand(pipelinesAnalyzeCmd)
	pipelinesCmd.AddCommand(pipelinesWatchCmd)
	rootCmd.AddCommand(pipelinesCmd)
}

func runPipelinesList(_ *cobra.Command, _ []string) error {
	client, err := newClient()
	if err != nil {
		return err
	}
	proj, err := resolveProject()
	if err != nil {
		return err
	}

	pipelines, err := client.ListPipelines(context.Background(), proj)
	if err != nil {
		return fmt.Errorf("list pipelines: %w", err)
	}

	if len(pipelines) == 0 {
		fmt.Printf("No pipelines found in project %q.\n", proj)
		return nil
	}

	switch flagOutput {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pipelines)
	case "plain":
		for _, p := range pipelines {
			fmt.Printf("%d\t%s\t%s\t%s\n", p.ID, p.Name, p.Folder, p.RepoName)
		}
	default:
		rows := make([][]string, len(pipelines))
		for i, p := range pipelines {
			folder := p.Folder
			if folder == "" || folder == "\\" {
				folder = "/"
			}
			rows[i] = []string{fmt.Sprintf("%d", p.ID), p.Name, folder, p.RepoName}
		}
		fmt.Print(ui.RenderTable(
			[]string{"ID", "NAME", "FOLDER", "REPOSITORY"},
			rows,
			[]int{8, 42, 22, 32},
		))
	}
	return nil
}

func runPipelinesRuns(_ *cobra.Command, args []string) error {
	pipelineID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("pipeline ID must be a number, got %q", args[0])
	}

	client, err := newClient()
	if err != nil {
		return err
	}
	proj, err := resolveProject()
	if err != nil {
		return err
	}

	runs, err := client.GetPipelineRuns(context.Background(), proj, pipelineID, runsLimit)
	if err != nil {
		return fmt.Errorf("get runs: %w", err)
	}

	if len(runs) == 0 {
		fmt.Printf("No runs found for pipeline %d.\n", pipelineID)
		return nil
	}

	switch flagOutput {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(runs)
	case "plain":
		for _, r := range runs {
			fmt.Printf("%d\t%s\t%s\t%s\t%s\t%s\n",
				r.ID, r.BuildNumber, r.State, r.Result, formatDuration(r.Duration), r.Branch)
		}
	default:
		rows := make([][]string, len(runs))
		for i, r := range runs {
			rows[i] = []string{
				fmt.Sprintf("%d", r.ID),
				r.BuildNumber,
				r.State,
				ui.ResultBadge(r.Result),
				formatDuration(r.Duration),
				r.Branch,
				formatTime(r.StartTime),
			}
		}
		fmt.Print(ui.RenderTable(
			[]string{"ID", "BUILD#", "STATE", "RESULT", "DURATION", "BRANCH", "STARTED"},
			rows,
			[]int{8, 12, 12, 22, 10, 28, 17},
		))
	}
	return nil
}

func runPipelinesAnalyze(_ *cobra.Command, args []string) error {
	pipelineID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("pipeline ID must be a number, got %q", args[0])
	}

	client, err := newClient()
	if err != nil {
		return err
	}
	proj, err := resolveProject()
	if err != nil {
		return err
	}

	ctx := context.Background()
	runs, err := client.GetPipelineRuns(ctx, proj, pipelineID, analyzeLimit)
	if err != nil {
		return fmt.Errorf("get runs: %w", err)
	}

	if len(runs) == 0 {
		fmt.Printf("No runs found for pipeline %d.\n", pipelineID)
		return nil
	}

	stats := analysis.ComputeStats(runs)

	// Collect stage failure data from failed/partially-succeeded runs (best-effort).
	stageMap := map[string]*analysis.StageStat{}
	for _, r := range runs {
		if r.Result != "failed" && r.Result != "partiallySucceeded" {
			continue
		}
		stages, err := client.GetBuildTimeline(ctx, proj, r.ID)
		if err != nil {
			continue
		}
		for _, s := range stages {
			if s.RecordType != "Stage" {
				continue
			}
			if _, ok := stageMap[s.Name]; !ok {
				stageMap[s.Name] = &analysis.StageStat{Name: s.Name}
			}
			stageMap[s.Name].Executions++
			if s.Result == "failed" {
				stageMap[s.Name].Failures++
			}
		}
	}

	stageSlice := make([]analysis.StageStat, 0, len(stageMap))
	for _, s := range stageMap {
		stageSlice = append(stageSlice, *s)
	}

	topStage := analysis.TopFailingStage(stageSlice)
	flakyStages := analysis.FlakyStages(stageSlice)

	switch flagOutput {
	case "json":
		out := map[string]interface{}{
			"totalRuns":       stats.TotalRuns,
			"successCount":    stats.SuccessCount,
			"failureCount":    stats.FailureCount,
			"canceledCount":   stats.CanceledCount,
			"avgDurationSec":  stats.AvgDuration.Seconds(),
			"failureRate":     stats.FailureRate,
			"topFailingStage": topStage,
			"flakyStages":     flakyStages,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)

	case "plain":
		fmt.Printf("total_runs=%d avg_duration=%s failure_rate=%.1f%% top_stage=%s\n",
			stats.TotalRuns, formatDuration(stats.AvgDuration), stats.FailureRate*100, topStage)

	default:
		fmt.Printf("Total runs:        %d (last %d)\n", stats.TotalRuns, analyzeLimit)
		fmt.Printf("Avg duration:      %s\n", formatDuration(stats.AvgDuration))
		nonCanceled := stats.TotalRuns - stats.CanceledCount
		fmt.Printf("Failure rate:      %.1f%% (%d of %d non-canceled)\n",
			stats.FailureRate*100, stats.FailureCount, nonCanceled)
		if topStage != "" {
			fmt.Printf("Top failing stage: %s\n", topStage)
		}
		if len(flakyStages) > 0 {
			fmt.Println("\nFlaky stages:")
			rows := make([][]string, len(flakyStages))
			for i, s := range flakyStages {
				rows[i] = []string{
					s.Name,
					fmt.Sprintf("%d", s.Failures),
					fmt.Sprintf("%d", s.Executions),
					fmt.Sprintf("%.1f%%", s.FailureRate()*100),
				}
			}
			fmt.Print(ui.RenderTable(
				[]string{"STAGE", "FAILURES", "EXECUTIONS", "FAILURE RATE"},
				rows,
				[]int{32, 10, 12, 14},
			))
		}
	}
	return nil
}

func runPipelinesWatch(_ *cobra.Command, args []string) error {
	pipelineID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("pipeline ID must be a number, got %q", args[0])
	}

	client, err := newClient()
	if err != nil {
		return err
	}
	proj, err := resolveProject()
	if err != nil {
		return err
	}

	m := ui.NewWatchModel(client, proj, pipelineID, time.Duration(watchInterval)*time.Second)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}

	if wm, ok := finalModel.(ui.WatchModel); ok {
		if wm.Err() != nil {
			return wm.Err()
		}
		if result := wm.FinalResult(); result != "" {
			fmt.Printf("Run completed: %s\n", result)
		}
	}
	return nil
}
