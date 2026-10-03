package runner_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ineslino/azpipe/internal/azdo"
	"github.com/ineslino/azpipe/internal/runner"
)

func TestReadinessBatch500And501(t *testing.T) {
	for _, count := range []int{500, 501} {
		client := &azdo.MockClient{}
		service := runner.NewService(client, "qa")
		var notified atomic.Int32
		service.OnPreview = func(_ int, _ runner.Review) { notified.Add(1) }
		selections := make([]runner.Selection, count)
		for i := range selections {
			selections[i] = runner.Selection{Pipeline: azdo.Pipeline{ID: i + 1}, Mode: runner.ModeRun}
		}
		reviews := service.PreviewAll(context.Background(), selections, 4)
		if len(reviews) != count || int(notified.Load()) != count {
			t.Fatal("lost or duplicate preview notification")
		}
		if count == 500 && len(client.PreviewRequests) != count {
			t.Fatal("valid boundary was rejected")
		}
		if count == 501 {
			if len(client.PreviewRequests) != 0 {
				t.Fatal("oversized batch made remote calls")
			}
			if _, err := service.QueueAll(context.Background(), reviews, 4); err == nil || len(client.QueueRequests) != 0 {
				t.Fatal("oversized batch was queued")
			}
		}
	}
}

type fourActive struct {
	azdo.MockClient
	started chan struct{}
}

func (c *fourActive) PreviewPipeline(ctx context.Context, _ string, _ azdo.RunRequest) error {
	c.started <- struct{}{}
	<-ctx.Done()
	return ctx.Err()
}

func TestReadinessCancellationReachesFourActivePreviews(t *testing.T) {
	client := &fourActive{started: make(chan struct{}, 4)}
	service := runner.NewService(client, "qa")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	selections := make([]runner.Selection, 4)
	for i := range selections {
		selections[i] = runner.Selection{Pipeline: azdo.Pipeline{ID: i + 1}, Mode: runner.ModeRun}
	}
	done := make(chan []runner.Review, 1)
	go func() { done <- service.PreviewAll(ctx, selections, 4) }()
	for range 4 {
		select {
		case <-client.started:
		case <-time.After(time.Second):
			t.Fatal("four previews were not active")
		}
	}
	cancel()
	select {
	case reviews := <-done:
		for _, r := range reviews {
			if r.State != runner.ReviewError || r.Err == nil {
				t.Fatal("cancelled operation reported ready")
			}
		}
	case <-time.After(time.Second):
		t.Fatal("preview cancellation did not finish")
	}
}
