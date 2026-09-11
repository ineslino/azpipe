package azdo

import (
	"context"
	"sync"
)

// MockClient is a test double for Client. Set the exported fields before use.
type MockClient struct {
	Projects           []Project
	Repos              []Repository
	Pipelines          []Pipeline
	PipelinesByProject map[string][]Pipeline
	Runs               []PipelineRun
	ActiveRun          *PipelineRun
	Timeline           []StageResult
	Err                error

	PreviewRequests      []RunRequest
	QueueRequests        []RunRequest
	ListPipelineProjects []string
	PreviewProjects      []string
	QueueProjects        []string
	RunProjects          []string
	QueuedRuns           []PipelineRun
	RunByID              map[int]PipelineRun
	PreviewErr           error
	QueueErr             error
	GetRunErr            error

	mu        sync.Mutex
	queueNext int
}

func (m *MockClient) ListProjects(_ context.Context) ([]Project, error) {
	return m.Projects, m.Err
}

func (m *MockClient) ListRepositories(_ context.Context, _ string) ([]Repository, error) {
	return m.Repos, m.Err
}

func (m *MockClient) ListPipelines(_ context.Context, project string) ([]Pipeline, error) {
	m.mu.Lock()
	m.ListPipelineProjects = append(m.ListPipelineProjects, project)
	m.mu.Unlock()
	pipelines := m.Pipelines
	if projectPipelines, ok := m.PipelinesByProject[project]; ok {
		pipelines = projectPipelines
	}
	result := make([]Pipeline, len(pipelines))
	for i, pipeline := range pipelines {
		result[i] = pipeline
		if result[i].Project == "" {
			result[i].Project = project
		}
	}
	return result, m.Err
}

func (m *MockClient) GetPipelineRuns(_ context.Context, _ string, _ int, limit int) ([]PipelineRun, error) {
	if limit > 0 && limit < len(m.Runs) {
		return m.Runs[:limit], m.Err
	}
	return m.Runs, m.Err
}

func (m *MockClient) GetActiveRun(_ context.Context, _ string, _ int) (*PipelineRun, error) {
	return m.ActiveRun, m.Err
}

func (m *MockClient) GetBuildTimeline(_ context.Context, _ string, _ int) ([]StageResult, error) {
	return m.Timeline, m.Err
}

func (m *MockClient) GetRepoPipelines(_ context.Context, _ string, _ string) ([]Pipeline, error) {
	return m.Pipelines, m.Err
}

func (m *MockClient) PreviewPipeline(_ context.Context, project string, request RunRequest) error {
	return m.previewPipeline(request, project)
}

func (m *MockClient) previewPipeline(request RunRequest, project string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PreviewRequests = append(m.PreviewRequests, cloneRunRequest(request))
	m.PreviewProjects = append(m.PreviewProjects, project)
	if m.PreviewErr != nil {
		return m.PreviewErr
	}
	return m.Err
}

func (m *MockClient) QueuePipeline(_ context.Context, project string, request RunRequest) (PipelineRun, error) {
	return m.queuePipeline(request, project)
}

func (m *MockClient) queuePipeline(request RunRequest, project string) (PipelineRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.QueueRequests = append(m.QueueRequests, cloneRunRequest(request))
	m.QueueProjects = append(m.QueueProjects, project)
	if m.QueueErr != nil {
		return PipelineRun{}, m.QueueErr
	}
	if m.Err != nil {
		return PipelineRun{}, m.Err
	}
	if m.queueNext >= len(m.QueuedRuns) {
		return PipelineRun{}, nil
	}
	run := m.QueuedRuns[m.queueNext]
	m.queueNext++
	return run, nil
}

func (m *MockClient) GetPipelineRun(_ context.Context, project string, runID int) (PipelineRun, error) {
	return m.getPipelineRun(runID, project)
}

func (m *MockClient) getPipelineRun(runID int, project string) (PipelineRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RunProjects = append(m.RunProjects, project)
	if m.GetRunErr != nil {
		return PipelineRun{}, m.GetRunErr
	}
	if m.Err != nil {
		return PipelineRun{}, m.Err
	}
	return m.RunByID[runID], nil
}

func cloneRunRequest(request RunRequest) RunRequest {
	clone := RunRequest{
		PipelineID:        request.PipelineID,
		Branch:            request.Branch,
		Commit:            request.Commit,
		DefinitionVersion: request.DefinitionVersion,
		PreviewHash:       request.PreviewHash,
		Parameters:        make(map[string]string, len(request.Parameters)),
	}
	for key, value := range request.Parameters {
		clone.Parameters[key] = value
	}
	return clone
}
