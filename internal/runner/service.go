package runner

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ineslino/azpipe/internal/azdo"
)

const (
	operationTimeout = 30 * time.Second
	maxParallel      = 4
)

var ErrPreviewIncomplete = errors.New("pipeline preview is incomplete")

// Service coordinates preview, queue, and refresh operations for a project.
type Service struct {
	client  azdo.Client
	project string
}

func NewService(client azdo.Client, project string) Service {
	return Service{client: client, project: project}
}

// PreviewAll previews every selection and returns reviews in selection order.
func (s Service) PreviewAll(ctx context.Context, selections []Selection, parallel int) []Review {
	reviews := make([]Review, len(selections))
	runParallel(len(selections), parallel, func(index int) {
		selection := selections[index]
		operationCtx, cancel := context.WithTimeout(ctx, operationTimeout)
		err := s.client.PreviewPipeline(operationCtx, s.project, selection.Request())
		cancel()

		reviews[index] = Review{Selection: selection, State: ReviewReady, Err: err}
		if err != nil {
			reviews[index].State = ReviewError
		}
	})
	return reviews
}

// QueueAll queues only fully previewed selections. It finishes every permitted
// queue request before reporting any partial failures.
func (s Service) QueueAll(ctx context.Context, reviews []Review, parallel int) ([]RunResult, error) {
	for _, review := range reviews {
		if review.State != ReviewReady || review.Err != nil {
			return nil, ErrPreviewIncomplete
		}
	}

	runs := make([]RunResult, len(reviews))
	runParallel(len(reviews), parallel, func(index int) {
		review := reviews[index]
		operationCtx, cancel := context.WithTimeout(ctx, operationTimeout)
		run, err := s.client.QueuePipeline(operationCtx, s.project, review.Selection.Request())
		cancel()
		runs[index] = RunResult{Review: review, Run: run, Err: err}
	})

	var errs []error
	for _, result := range runs {
		if result.Err != nil {
			errs = append(errs, result.Err)
		}
	}
	return runs, errors.Join(errs...)
}

// Refresh fetches the latest state for queued non-terminal runs in result order.
func (s Service) Refresh(ctx context.Context, runs []RunResult, parallel int) []RunResult {
	refreshed := append([]RunResult(nil), runs...)
	runParallel(len(refreshed), parallel, func(index int) {
		result := refreshed[index]
		if result.Err != nil || result.Run.ID == 0 || result.Run.State == "completed" {
			return
		}

		operationCtx, cancel := context.WithTimeout(ctx, operationTimeout)
		run, err := s.client.GetPipelineRun(operationCtx, s.project, result.Run.ID)
		cancel()
		if err != nil {
			refreshed[index].Err = err
			return
		}
		refreshed[index].Run = run
	})
	return refreshed
}

func runParallel(items, parallel int, operation func(int)) {
	if items == 0 {
		return
	}
	workers := parallel
	if workers < 1 {
		workers = 1
	}
	if workers > maxParallel {
		workers = maxParallel
	}
	if workers > items {
		workers = items
	}

	jobs := make(chan int)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for index := range jobs {
				operation(index)
			}
		}()
	}
	for index := range items {
		jobs <- index
	}
	close(jobs)
	group.Wait()
}
