package runner_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ineslino/azpipe/internal/azdo"
	"github.com/ineslino/azpipe/internal/runner"
)

func TestSelectionParameters_PlanAddsPlanOnly(t *testing.T) {
	selection := runner.Selection{
		Pipeline: azdo.Pipeline{ID: 42, PlanContract: &azdo.PlanContract{Parameter: "planOnly", PlanValue: "true", RunValue: "false"}},
		Mode:     runner.ModePlan,
		Branch:   "main",
	}

	if got := selection.Parameters()["planOnly"]; got != "true" {
		t.Fatalf("plan selection must request a plan-only run, got %q", got)
	}
}

func TestSelectionParameters_RunOmitsPlanOnly(t *testing.T) {
	selection := runner.Selection{Pipeline: azdo.Pipeline{ID: 42}, Mode: runner.ModeRun}

	if _, ok := selection.Parameters()["planOnly"]; ok {
		t.Fatal("normal run must not send planOnly=false")
	}
}

func TestSelectionRequest_DefaultsBranchToMain(t *testing.T) {
	selection := runner.Selection{Pipeline: azdo.Pipeline{ID: 42}, Mode: runner.ModeRun}

	if got := selection.Request().Branch; got != "main" {
		t.Fatalf("empty selection branch must request main, got %q", got)
	}
}

func TestSelectionIdentity_IsPipelineID(t *testing.T) {
	selection := runner.Selection{Pipeline: azdo.Pipeline{ID: 42, Name: "deploy"}}

	if got := selection.ID(); got != 42 {
		t.Fatalf("selection identity must be pipeline ID, got %d", got)
	}
}

func TestServicePreviewAll_PreviewsEverySelectedPipelineInSelectionOrder(t *testing.T) {
	client := &azdo.MockClient{}
	service := runner.NewService(client, "sample-project")
	selections := []runner.Selection{
		{Pipeline: azdo.Pipeline{ID: 11}, Mode: runner.ModeRun},
		{Pipeline: azdo.Pipeline{ID: 22, PlanContract: &azdo.PlanContract{Parameter: "planOnly", PlanValue: "true", RunValue: "false"}}, Mode: runner.ModePlan},
		{Pipeline: azdo.Pipeline{ID: 33}, Mode: runner.ModeRun},
	}

	reviews := service.PreviewAll(context.Background(), selections, 2)

	if len(reviews) != 3 {
		t.Fatalf("got %d reviews, want 3", len(reviews))
	}
	for i, wantID := range []int{11, 22, 33} {
		if got := reviews[i].Selection.ID(); got != wantID {
			t.Errorf("review %d belongs to pipeline %d, want %d", i, got, wantID)
		}
		if reviews[i].State != runner.ReviewReady {
			t.Errorf("review %d state = %s, want READY", i, reviews[i].State)
		}
	}
	if len(client.PreviewRequests) != 3 {
		t.Fatalf("previewed %d pipelines, want 3", len(client.PreviewRequests))
	}
	previewedIDs := make([]int, len(client.PreviewRequests))
	for index, request := range client.PreviewRequests {
		previewedIDs[index] = request.PipelineID
	}
	slices.Sort(previewedIDs)
	if !slices.Equal(previewedIDs, []int{11, 22, 33}) {
		t.Fatalf("previewed pipeline IDs = %v, want [11 22 33]", previewedIDs)
	}
}

func TestServiceQueueAll_PreviewErrorPreventsEveryQueueCall(t *testing.T) {
	base := &azdo.MockClient{}
	client := previewFailureClient{MockClient: base, failedPipelineID: 22}
	service := runner.NewService(client, "sample-project")
	selections := []runner.Selection{
		{Pipeline: azdo.Pipeline{ID: 11}, Mode: runner.ModeRun},
		{Pipeline: azdo.Pipeline{ID: 22}, Mode: runner.ModePlan},
	}

	reviews := service.PreviewAll(context.Background(), selections, 2)
	runs, err := service.QueueAll(context.Background(), reviews, 2)

	if !errors.Is(err, runner.ErrPreviewIncomplete) {
		t.Fatalf("queue after a failed preview error = %v, want ErrPreviewIncomplete", err)
	}
	if runs != nil {
		t.Fatalf("queue after a failed preview returned %d runs, want nil", len(runs))
	}
	if len(base.QueueRequests) != 0 {
		t.Fatalf("queued %d pipelines despite failed preview", len(base.QueueRequests))
	}
}

func TestServiceQueueAll_ReadyReviewWithErrorPreventsEveryQueueCall(t *testing.T) {
	client := &azdo.MockClient{}
	service := runner.NewService(client, "sample-project")
	reviews := readyReviews(11, 22)
	reviews[1].Err = errors.New("preview rejected")

	runs, err := service.QueueAll(context.Background(), reviews, 2)

	if !errors.Is(err, runner.ErrPreviewIncomplete) {
		t.Fatalf("queue after ready review with error = %v, want ErrPreviewIncomplete", err)
	}
	if runs != nil {
		t.Fatalf("queue after ready review with error returned %d runs, want nil", len(runs))
	}
	if len(client.QueueRequests) != 0 {
		t.Fatalf("queued %d pipelines despite review error", len(client.QueueRequests))
	}
}

func TestServiceQueueAll_RespectsParallelLimitAndKeepsSelectionOrder(t *testing.T) {
	base := &azdo.MockClient{QueuedRuns: []azdo.PipelineRun{{ID: 101}, {ID: 202}, {ID: 303}, {ID: 404}}}
	client := &concurrentQueueClient{MockClient: base}
	service := runner.NewService(client, "sample-project")
	reviews := readyReviews(11, 22, 33, 44)

	runs, err := service.QueueAll(context.Background(), reviews, 2)

	if err != nil {
		t.Fatalf("QueueAll() error = %v", err)
	}
	if client.maxObserved > 2 {
		t.Fatalf("maximum queue concurrency = %d, want at most 2", client.maxObserved)
	}
	for i, wantID := range []int{11, 22, 33, 44} {
		if got := runs[i].Review.Selection.ID(); got != wantID {
			t.Errorf("run %d belongs to pipeline %d, want %d", i, got, wantID)
		}
	}
}

func TestServiceQueueAll_CapsRequestedParallelismAtFour(t *testing.T) {
	base := &azdo.MockClient{QueuedRuns: []azdo.PipelineRun{{ID: 101}, {ID: 202}, {ID: 303}, {ID: 404}, {ID: 505}}}
	client := &concurrentQueueClient{MockClient: base}
	service := runner.NewService(client, "sample-project")

	_, err := service.QueueAll(context.Background(), readyReviews(11, 22, 33, 44, 55), 5)

	if err != nil {
		t.Fatalf("QueueAll() error = %v", err)
	}
	if client.maxObserved > 4 {
		t.Fatalf("maximum queue concurrency = %d, want at most 4", client.maxObserved)
	}
}

func TestServiceQueueAll_PreservesSuccessfulRunsAfterPartialFailure(t *testing.T) {
	service := runner.NewService(partialQueueClient{Client: &azdo.MockClient{}, failedPipelineID: 22}, "sample-project")

	runs, err := service.QueueAll(context.Background(), readyReviews(11, 22, 33), 2)

	if err == nil {
		t.Fatal("partial queue failure must be returned")
	}
	if got := runs[0].Run.ID; got != 1100 {
		t.Errorf("first successful run ID = %d, want 1100", got)
	}
	if runs[1].Err == nil {
		t.Error("failed queue result must retain its error")
	}
	if got := runs[2].Run.ID; got != 3300 {
		t.Errorf("last successful run ID = %d, want 3300", got)
	}
}

func TestServiceRefresh_UpdatesOnlyNonTerminalRuns(t *testing.T) {
	client := &azdo.MockClient{RunByID: map[int]azdo.PipelineRun{7: {ID: 7, State: "completed", Result: "succeeded"}}}
	service := runner.NewService(client, "sample-project")
	runs := []runner.RunResult{
		{Review: runner.Review{Selection: runner.Selection{Pipeline: azdo.Pipeline{ID: 1}}}, Run: azdo.PipelineRun{ID: 7, State: "inProgress"}},
		{Review: runner.Review{Selection: runner.Selection{Pipeline: azdo.Pipeline{ID: 2}}}, Run: azdo.PipelineRun{ID: 8, State: "completed", Result: "failed"}},
	}

	got := service.Refresh(context.Background(), runs, 2)

	if got[0].Run.Result != "succeeded" {
		t.Errorf("refreshed run result = %q, want succeeded", got[0].Run.Result)
	}
	if got[1].Run.Result != "failed" {
		t.Errorf("terminal run result = %q, want failed", got[1].Run.Result)
	}
}

func readyReviews(ids ...int) []runner.Review {
	reviews := make([]runner.Review, len(ids))
	for i, id := range ids {
		reviews[i] = runner.Review{
			Selection: runner.Selection{Pipeline: azdo.Pipeline{ID: id}, Mode: runner.ModeRun},
			State:     runner.ReviewReady,
		}
	}
	return reviews
}

type previewFailureClient struct {
	*azdo.MockClient
	failedPipelineID int
}

func (c previewFailureClient) PreviewPipeline(ctx context.Context, project string, request azdo.RunRequest) error {
	if err := c.MockClient.PreviewPipeline(ctx, project, request); err != nil {
		return err
	}
	if request.PipelineID == c.failedPipelineID {
		return errors.New("preview rejected")
	}
	return nil
}

type concurrentQueueClient struct {
	*azdo.MockClient
	mu          sync.Mutex
	current     int
	maxObserved int
}

func (c *concurrentQueueClient) QueuePipeline(ctx context.Context, project string, request azdo.RunRequest) (azdo.PipelineRun, error) {
	c.mu.Lock()
	c.current++
	if c.current > c.maxObserved {
		c.maxObserved = c.current
	}
	c.mu.Unlock()

	time.Sleep(25 * time.Millisecond)

	c.mu.Lock()
	c.current--
	c.mu.Unlock()
	return c.MockClient.QueuePipeline(ctx, project, request)
}

type partialQueueClient struct {
	azdo.Client
	failedPipelineID int
}

func (c partialQueueClient) QueuePipeline(_ context.Context, _ string, request azdo.RunRequest) (azdo.PipelineRun, error) {
	if request.PipelineID == c.failedPipelineID {
		return azdo.PipelineRun{}, errors.New("queue rejected")
	}
	return azdo.PipelineRun{ID: request.PipelineID * 100}, nil
}
