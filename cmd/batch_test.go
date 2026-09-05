package cmd

import (
	"context"
	"github.com/ineslino/azpipe/internal/azdo"
	"github.com/spf13/cobra"
	"io"
	"testing"
)

func TestResumeOnlyReadsRunsAndReportsFailure(t *testing.T) {
	client := &azdo.MockClient{RunByID: map[int]azdo.PipelineRun{42: {ID: 42, State: "completed", Result: "failed"}}}
	j := batchJournal{Project: "sample", Runs: []batchRecord{{PipelineID: 1, Run: azdo.PipelineRun{ID: 42}}}}
	c := &cobra.Command{}
	c.SetOut(io.Discard)
	err := monitorJournal(context.Background(), c, client, &j, func() error { return nil })
	if err == nil || len(client.QueueRequests) != 0 {
		t.Fatal("resume must report failure without queue", err)
	}
}
