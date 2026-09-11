package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ineslino/azpipe/internal/azdo"
)

const (
	operationTimeout = 30 * time.Second
	maxParallel      = 4
)

var ErrPreviewIncomplete = errors.New("pipeline preview is incomplete")

// Service coordinates preview, queue, and refresh operations for a pipeline context.
type Service struct {
	client   azdo.Client
	project  string
	OnResult func(int, RunResult) error
}

func NewService(client azdo.Client, project string) Service {
	return Service{client: client, project: project}
}

func (s Service) Schema(ctx context.Context, id int, branch string) (azdo.ParameterSchema, error) {
	return s.schema(ctx, s.project, id, branch)
}

// SchemaForPipeline reads parameters using the project that owns the pipeline.
func (s Service) SchemaForPipeline(ctx context.Context, pipeline azdo.Pipeline, branch string) (azdo.ParameterSchema, error) {
	project, err := s.projectForPipeline(pipeline)
	if err != nil {
		return azdo.ParameterSchema{}, err
	}
	return s.schema(ctx, project, pipeline.ID, branch)
}

func (s Service) schema(ctx context.Context, project string, id int, branch string) (azdo.ParameterSchema, error) {
	provider, ok := s.client.(azdo.SchemaProvider)
	if !ok {
		return azdo.ParameterSchema{}, errors.New("cliente sem descoberta de parâmetros YAML")
	}
	if strings.TrimSpace(project) == "" || project == AllProjects {
		return azdo.ParameterSchema{}, errors.New("projecto da pipeline não está definido")
	}
	return provider.GetPipelineSchema(ctx, project, id, branch)
}

// PreviewAll previews every selection and returns reviews in selection order.
func (s Service) PreviewAll(ctx context.Context, selections []Selection, parallel int) []Review {
	reviews := make([]Review, len(selections))
	runParallel(len(selections), parallel, func(index int) {
		selection := selections[index]
		operationCtx, cancel := context.WithTimeout(ctx, operationTimeout)
		project, projectErr := s.projectForPipeline(selection.Pipeline)
		request := selection.Request()
		err := projectErr
		var schema *azdo.ParameterSchema
		if err == nil {
			if provider, ok := s.client.(azdo.SchemaProvider); ok {
				loaded, loadErr := provider.GetPipelineSchema(operationCtx, project, selection.ID(), selection.Branch)
				err = loadErr
				if err == nil {
					schema = &loaded
					err = loaded.Validate(request.Parameters)
				}
			}
		}
		if selection.Mode == ModePlan && selection.Pipeline.PlanContract == nil && err == nil {
			err = errors.New("PLAN indisponível: contrato validado em falta")
		}
		if prepare, ok := s.client.(interface {
			PrepareRun(context.Context, string, azdo.RunRequest) (azdo.RunRequest, error)
		}); ok && err == nil {
			request, err = prepare.PrepareRun(operationCtx, project, request)
			if err == nil && schema != nil && (schema.Commit != request.Commit || schema.DefinitionVersion != request.DefinitionVersion) {
				err = errors.New("fonte alterada durante revisão; repetir preview")
			}
		}
		if err == nil {
			err = s.client.PreviewPipeline(operationCtx, project, request)
		}
		cancel()

		reviews[index] = Review{Selection: selection, Request: request, State: ReviewReady, Err: err}
		if err != nil {
			reviews[index].State = ReviewError
		}
	})
	return reviews
}

// QueueAll queues only fully previewed selections. It finishes every permitted
// queue request before reporting any partial failures.
func (s Service) QueueAll(ctx context.Context, reviews []Review, parallel int) ([]RunResult, error) {
	if len(reviews) == 0 {
		return nil, ErrPreviewIncomplete
	}
	for _, review := range reviews {
		if review.State != ReviewReady || review.Err != nil {
			return nil, ErrPreviewIncomplete
		}
	}
	for _, review := range reviews {
		if review.Request.PreviewHash != "" {
			project, projectErr := s.projectForPipeline(review.Selection.Pipeline)
			if projectErr != nil {
				return nil, errors.Join(ErrPreviewIncomplete, projectErr)
			}
			operation, cancel := context.WithTimeout(ctx, operationTimeout)
			err := s.client.PreviewPipeline(operation, project, review.Request)
			cancel()
			if err != nil {
				return nil, errors.Join(ErrPreviewIncomplete, err)
			}
		}
	}

	runs := make([]RunResult, len(reviews))
	runParallel(len(reviews), parallel, func(index int) {
		review := reviews[index]
		operationCtx, cancel := context.WithTimeout(ctx, operationTimeout)
		project, projectErr := s.projectForPipeline(review.Selection.Pipeline)
		request := review.Request
		if request.PipelineID == 0 {
			request = review.Selection.Request()
		}
		var run azdo.PipelineRun
		err := projectErr
		if err == nil {
			run, err = s.client.QueuePipeline(operationCtx, project, request)
		}
		cancel()
		runs[index] = RunResult{Review: review, Run: run, Err: err}
		if s.OnResult != nil {
			if persistErr := s.OnResult(index, runs[index]); persistErr != nil {
				runs[index].Err = errors.Join(runs[index].Err, persistErr)
			}
		}
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
		project, projectErr := s.projectForPipeline(result.Review.Selection.Pipeline)
		if projectErr != nil {
			refreshed[index].Err = projectErr
			cancel()
			return
		}
		run, err := s.client.GetPipelineRun(operationCtx, project, result.Run.ID)
		cancel()
		if err != nil {
			refreshed[index].Err = err
			return
		}
		refreshed[index].Run = run
	})
	return refreshed
}

func (s Service) projectForPipeline(pipeline azdo.Pipeline) (string, error) {
	project := strings.TrimSpace(pipeline.Project)
	if project == "" {
		project = strings.TrimSpace(s.project)
	}
	if project == "" || project == AllProjects {
		return "", fmt.Errorf("projecto da pipeline %d não está definido", pipeline.ID)
	}
	return project, nil
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
