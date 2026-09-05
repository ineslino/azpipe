package runner

import (
	"context"
	"encoding/json"
	"github.com/ineslino/azpipe/internal/azdo"
	"os"
	"testing"
)

type pinClient struct{ azdo.MockClient }

func (c *pinClient) PrepareRun(_ context.Context, _ string, r azdo.RunRequest) (azdo.RunRequest, error) {
	r.Commit = "1234567890123456789012345678901234567890"
	r.DefinitionVersion = 7
	return r, nil
}

func TestPreparedRequestSurvivesReview(t *testing.T) {
	c := &pinClient{}
	s := NewService(c, "example")
	reviews := s.PreviewAll(context.Background(), []Selection{{Pipeline: azdo.Pipeline{ID: 1}, Mode: ModeRun}}, 1)
	_, err := s.QueueAll(context.Background(), reviews, 1)
	if err != nil || len(c.QueueRequests) != 1 || c.QueueRequests[0].Commit != reviews[0].Request.Commit || c.QueueRequests[0].DefinitionVersion != 7 {
		t.Fatal("pinned request lost", err, c.QueueRequests)
	}
}

func TestPlanWithoutContractNeverPreviewsOrQueues(t *testing.T) {
	c := &azdo.MockClient{}
	s := NewService(c, "example")
	r := s.PreviewAll(context.Background(), []Selection{{Pipeline: azdo.Pipeline{ID: 1}, Mode: ModePlan}}, 1)
	if _, err := s.QueueAll(context.Background(), r, 1); err == nil || len(c.PreviewRequests) != 0 || len(c.QueueRequests) != 0 {
		t.Fatal("unknown PLAN accepted")
	}
}

func TestRunExplicitlyReversesPlanParameter(t *testing.T) {
	s := Selection{Pipeline: azdo.Pipeline{PlanContract: &azdo.PlanContract{Parameter: "action", PlanValue: "preview", RunValue: "apply"}}, Mode: ModeRun, Inputs: map[string]string{"region": "demo", "action": "preview"}}
	if s.Parameters()["action"] != "apply" || s.Parameters()["region"] != "demo" {
		t.Fatal(s.Parameters())
	}
}

func TestJournalPreservesAcceptedAndUnknown(t *testing.T) {
	t.Setenv("AZPIPE_DATA_DIR", t.TempDir())
	j, err := NewJournal("https://dev.azure.com/example", "sample", []Review{{Selection: Selection{Pipeline: azdo.Pipeline{ID: 1}}}, {Selection: Selection{Pipeline: azdo.Pipeline{ID: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Record(0, RunResult{Run: azdo.PipelineRun{ID: 42}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(j.Path())
	if err != nil {
		t.Fatal(err)
	}
	var saved Journal
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(j.Path())
	if saved.Runs[0].Run.ID != 42 || saved.Runs[1].Error == "" || info.Mode().Perm() != 0600 {
		t.Fatal("journal state or permissions incorrect")
	}
}
