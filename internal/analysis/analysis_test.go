package analysis_test

import (
	"testing"
	"time"

	"github.com/ineslino/azpipe/internal/analysis"
	"github.com/ineslino/azpipe/internal/azdo"
)

func TestComputeStats_Empty(t *testing.T) {
	stats := analysis.ComputeStats(nil)
	if stats.TotalRuns != 0 || stats.FailureRate != 0 || stats.AvgDuration != 0 {
		t.Errorf("expected zero stats for empty input, got %+v", stats)
	}
}

func TestComputeStats_AllSucceeded(t *testing.T) {
	runs := []azdo.PipelineRun{
		{Result: "succeeded", Duration: 2 * time.Minute},
		{Result: "succeeded", Duration: 4 * time.Minute},
	}
	s := analysis.ComputeStats(runs)

	if s.TotalRuns != 2 {
		t.Errorf("TotalRuns: want 2, got %d", s.TotalRuns)
	}
	if s.SuccessCount != 2 {
		t.Errorf("SuccessCount: want 2, got %d", s.SuccessCount)
	}
	if s.FailureCount != 0 {
		t.Errorf("FailureCount: want 0, got %d", s.FailureCount)
	}
	if s.FailureRate != 0 {
		t.Errorf("FailureRate: want 0, got %f", s.FailureRate)
	}
	if s.AvgDuration != 3*time.Minute {
		t.Errorf("AvgDuration: want 3m, got %s", s.AvgDuration)
	}
}

func TestComputeStats_MixedResults(t *testing.T) {
	runs := []azdo.PipelineRun{
		{Result: "succeeded", Duration: 2 * time.Minute},
		{Result: "failed", Duration: 1 * time.Minute},
		{Result: "canceled", Duration: 0},
	}
	s := analysis.ComputeStats(runs)

	if s.TotalRuns != 3 {
		t.Errorf("TotalRuns: want 3, got %d", s.TotalRuns)
	}
	if s.SuccessCount != 1 {
		t.Errorf("SuccessCount: want 1, got %d", s.SuccessCount)
	}
	if s.FailureCount != 1 {
		t.Errorf("FailureCount: want 1, got %d", s.FailureCount)
	}
	if s.CanceledCount != 1 {
		t.Errorf("CanceledCount: want 1, got %d", s.CanceledCount)
	}
	// failure rate = 1 failure / 2 non-canceled = 0.5
	if s.FailureRate != 0.5 {
		t.Errorf("FailureRate: want 0.5, got %f", s.FailureRate)
	}
	// avg duration from 2 completed runs
	if s.AvgDuration != 90*time.Second {
		t.Errorf("AvgDuration: want 1m30s, got %s", s.AvgDuration)
	}
}

func TestComputeStats_AllCanceled(t *testing.T) {
	runs := []azdo.PipelineRun{
		{Result: "canceled"},
		{Result: "canceled"},
	}
	s := analysis.ComputeStats(runs)
	if s.FailureRate != 0 {
		t.Errorf("FailureRate: want 0 when all canceled, got %f", s.FailureRate)
	}
}

func TestComputeStats_PartiallySucceeded(t *testing.T) {
	runs := []azdo.PipelineRun{
		{Result: "partiallySucceeded", Duration: 3 * time.Minute},
		{Result: "failed", Duration: 2 * time.Minute},
	}
	s := analysis.ComputeStats(runs)
	if s.SuccessCount != 1 {
		t.Errorf("partiallySucceeded should count as success, got SuccessCount=%d", s.SuccessCount)
	}
	if s.FailureCount != 1 {
		t.Errorf("FailureCount: want 1, got %d", s.FailureCount)
	}
}

func TestTopFailingStage(t *testing.T) {
	stages := []analysis.StageStat{
		{Name: "Build", Executions: 5, Failures: 1},
		{Name: "Test", Executions: 5, Failures: 3},
		{Name: "Deploy", Executions: 5, Failures: 2},
	}
	got := analysis.TopFailingStage(stages)
	if got != "Test" {
		t.Errorf("TopFailingStage: want Test, got %q", got)
	}
}

func TestTopFailingStage_Empty(t *testing.T) {
	got := analysis.TopFailingStage(nil)
	if got != "" {
		t.Errorf("expected empty string for nil input, got %q", got)
	}
}

func TestFlakyStages(t *testing.T) {
	stages := []analysis.StageStat{
		{Name: "Build", Executions: 5, Failures: 0},      // never fails — not flaky
		{Name: "Test", Executions: 5, Failures: 2},        // 40% fail rate — flaky
		{Name: "Deploy", Executions: 5, Failures: 5},      // always fails — not flaky
		{Name: "Integration", Executions: 2, Failures: 1}, // < 3 executions — excluded
		{Name: "E2E", Executions: 4, Failures: 1},         // 25% fail rate — flaky
	}
	got := analysis.FlakyStages(stages)

	if len(got) != 2 {
		t.Fatalf("FlakyStages: want 2 results, got %d: %v", len(got), got)
	}
	// sorted by failure rate desc: Test (40%) then E2E (25%)
	if got[0].Name != "Test" {
		t.Errorf("first flaky stage: want Test, got %q", got[0].Name)
	}
	if got[1].Name != "E2E" {
		t.Errorf("second flaky stage: want E2E, got %q", got[1].Name)
	}
}

func TestStageStat_FailureRate(t *testing.T) {
	cases := []struct {
		s    analysis.StageStat
		want float64
	}{
		{analysis.StageStat{Executions: 0, Failures: 0}, 0},
		{analysis.StageStat{Executions: 4, Failures: 1}, 0.25},
		{analysis.StageStat{Executions: 3, Failures: 3}, 1.0},
	}
	for _, c := range cases {
		got := c.s.FailureRate()
		if got != c.want {
			t.Errorf("FailureRate(%+v): want %f, got %f", c.s, c.want, got)
		}
	}
}
