package ui

import (
	"github.com/ineslino/azpipe/internal/azdo"
	"testing"
	"time"
)

func TestWatchQueuedRunDoesNotComplete(t *testing.T) {
	c := &azdo.MockClient{Runs: []azdo.PipelineRun{{ID: 42, State: "notStarted"}}}
	m := NewWatchModel(c, "sample", 1, time.Second)
	if _, ok := m.fetchStatus()().(runCompleteMsg); ok {
		t.Fatal("queued run treated as completed")
	}
}
