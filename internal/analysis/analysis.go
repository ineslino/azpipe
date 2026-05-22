package analysis

import (
	"sort"
	"time"

	"github.com/ineslino/azpipe/internal/azdo"
)

// Stats summarises pipeline health metrics computed from a set of runs.
type Stats struct {
	TotalRuns     int
	SuccessCount  int
	FailureCount  int
	CanceledCount int
	AvgDuration   time.Duration
	FailureRate   float64 // fraction 0.0–1.0; excludes canceled runs from denominator
}

// StageStat accumulates per-stage execution and failure counts across runs.
type StageStat struct {
	Name       string `json:"name"`
	Executions int    `json:"executions"`
	Failures   int    `json:"failures"`
}

// FailureRate returns the fraction of executions that failed.
func (s StageStat) FailureRate() float64 {
	if s.Executions == 0 {
		return 0
	}
	return float64(s.Failures) / float64(s.Executions)
}

// IsFlaky returns true for stages that sometimes fail but not always,
// with at least 3 executions to reduce noise.
func (s StageStat) IsFlaky() bool {
	rate := s.FailureRate()
	return rate > 0 && rate < 1.0 && s.Executions >= 3
}

// ComputeStats derives aggregate statistics from a slice of pipeline runs.
func ComputeStats(runs []azdo.PipelineRun) Stats {
	if len(runs) == 0 {
		return Stats{}
	}

	var stats Stats
	stats.TotalRuns = len(runs)

	var totalDuration time.Duration
	var withDuration int

	for _, r := range runs {
		switch r.Result {
		case "succeeded", "partiallySucceeded":
			stats.SuccessCount++
		case "failed":
			stats.FailureCount++
		case "canceled":
			stats.CanceledCount++
		}
		if r.Duration > 0 {
			totalDuration += r.Duration
			withDuration++
		}
	}

	if withDuration > 0 {
		stats.AvgDuration = totalDuration / time.Duration(withDuration)
	}

	denominator := stats.TotalRuns - stats.CanceledCount
	if denominator > 0 {
		stats.FailureRate = float64(stats.FailureCount) / float64(denominator)
	}

	return stats
}

// TopFailingStage returns the name of the stage with the most failures.
// Returns empty string when the slice is empty.
func TopFailingStage(stages []StageStat) string {
	var top StageStat
	for _, s := range stages {
		if s.Failures > top.Failures {
			top = s
		}
	}
	return top.Name
}

// FlakyStages returns stages whose failure rate is between 0 and 100%
// (i.e., they sometimes pass and sometimes fail) with ≥3 executions,
// sorted by failure rate descending.
func FlakyStages(stages []StageStat) []StageStat {
	var result []StageStat
	for _, s := range stages {
		if s.IsFlaky() {
			result = append(result, s)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].FailureRate() > result[j].FailureRate()
	})
	return result
}
