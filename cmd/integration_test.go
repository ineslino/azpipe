package cmd_test

// Integration test skeleton using MockClient.
// These tests exercise the command handlers end-to-end without hitting
// the real Azure DevOps API. Replace MockClient fixtures with richer
// data or a recorded-fixture httptest.Server as the suite matures.

import (
	"context"
	"testing"
	"time"

	"github.com/ineslino/azpipe/internal/analysis"
	"github.com/ineslino/azpipe/internal/azdo"
)

// TestMockClientListProjects verifies the MockClient satisfies the Client
// interface contract and returns the data we set.
func TestMockClientListProjects(t *testing.T) {
	mock := &azdo.MockClient{
		Projects: []azdo.Project{
			{ID: "abc-123", Name: "Platform", URL: "https://dev.azure.com/myorg/Platform"},
			{ID: "def-456", Name: "Services", URL: "https://dev.azure.com/myorg/Services"},
		},
	}

	projects, err := mock.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("want 2 projects, got %d", len(projects))
	}
	if projects[0].Name != "Platform" {
		t.Errorf("projects[0].Name: want Platform, got %q", projects[0].Name)
	}
}

func TestMockClientListPipelines(t *testing.T) {
	mock := &azdo.MockClient{
		Pipelines: []azdo.Pipeline{
			{ID: 1, Name: "Build", Folder: "\\", RepoName: "myrepo"},
			{ID: 2, Name: "Deploy", Folder: "\\infra", RepoName: "myrepo"},
		},
	}

	pipelines, err := mock.ListPipelines(context.Background(), "myproject")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pipelines) != 2 {
		t.Fatalf("want 2 pipelines, got %d", len(pipelines))
	}
}

func TestMockClientGetPipelineRunsLimit(t *testing.T) {
	runs := make([]azdo.PipelineRun, 10)
	for i := range runs {
		runs[i] = azdo.PipelineRun{ID: i + 1, Result: "succeeded", Duration: time.Minute}
	}
	mock := &azdo.MockClient{Runs: runs}

	got, err := mock.GetPipelineRuns(context.Background(), "proj", 1, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("want 5 runs (limit applied), got %d", len(got))
	}
}

func TestMockClientGetActiveRun_None(t *testing.T) {
	mock := &azdo.MockClient{} // no active run set
	run, err := mock.GetActiveRun(context.Background(), "proj", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run != nil {
		t.Errorf("expected nil active run, got %+v", run)
	}
}

func TestMockClientGetActiveRun_Set(t *testing.T) {
	active := &azdo.PipelineRun{ID: 99, State: "inProgress"}
	mock := &azdo.MockClient{ActiveRun: active}
	run, err := mock.GetActiveRun(context.Background(), "proj", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run == nil || run.ID != 99 {
		t.Errorf("expected run ID 99, got %v", run)
	}
}

// TestAnalysisPipelineRoundTrip exercises the full stats+stage pipeline
// using mock data, simulating what runPipelinesAnalyze does.
func TestAnalysisPipelineRoundTrip(t *testing.T) {
	runs := []azdo.PipelineRun{
		{ID: 1, Result: "succeeded", Duration: 3 * time.Minute},
		{ID: 2, Result: "failed", Duration: 1 * time.Minute},
		{ID: 3, Result: "failed", Duration: 2 * time.Minute},
		{ID: 4, Result: "succeeded", Duration: 4 * time.Minute},
		{ID: 5, Result: "canceled"},
	}
	timeline := []azdo.StageResult{
		{Name: "Build", RecordType: "Stage", State: "completed", Result: "succeeded"},
		{Name: "Test", RecordType: "Stage", State: "completed", Result: "failed"},
		{Name: "Deploy", RecordType: "Stage", State: "completed", Result: "skipped"},
	}
	mock := &azdo.MockClient{Runs: runs, Timeline: timeline}

	// Compute stats
	stats := analysis.ComputeStats(runs)
	if stats.TotalRuns != 5 {
		t.Errorf("TotalRuns: want 5, got %d", stats.TotalRuns)
	}
	if stats.FailureCount != 2 {
		t.Errorf("FailureCount: want 2, got %d", stats.FailureCount)
	}
	// failure rate = 2 / (5-1 canceled) = 0.5
	if stats.FailureRate != 0.5 {
		t.Errorf("FailureRate: want 0.5, got %f", stats.FailureRate)
	}

	// Collect stage failures as the analyze command would.
	ctx := context.Background()
	stageMap := map[string]*analysis.StageStat{}
	for _, r := range runs {
		if r.Result != "failed" && r.Result != "partiallySucceeded" {
			continue
		}
		stages, _ := mock.GetBuildTimeline(ctx, "proj", r.ID)
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

	top := analysis.TopFailingStage(stageSlice)
	if top != "Test" {
		t.Errorf("TopFailingStage: want Test, got %q", top)
	}
}
