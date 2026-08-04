package azdo

import (
	"context"
	"sync"
)

// MockClient is a test double for Client. Set the exported fields before use.
type MockClient struct {
	Projects  []Project
	Repos     []Repository
	Pipelines []Pipeline
	Runs      []PipelineRun
	ActiveRun *PipelineRun
	Timeline  []StageResult
	Err       error

	PreviewRequests []RunRequest
	QueueRequests   []RunRequest
	QueuedRuns      []PipelineRun
	RunByID         map[int]PipelineRun
	PreviewErr      error
	QueueErr        error
	GetRunErr       error

	mu        sync.Mutex
	queueNext int
}

func (m *MockClient) ListProjects(_ context.Context) ([]Project, error) {
	return m.Projects, m.Err
}

func (m *MockClient) ListRepositories(_ context.Context, _ string) ([]Repository, error) {
	return m.Repos, m.Err
}

func (m *MockClient) ListPipelines(_ context.Context, _ string) ([]Pipeline, error) {
	return m.Pipelines, m.Err
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

func (m *MockClient) PreviewPipeline(_ context.Context, _ string, request RunRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PreviewRequests = append(m.PreviewRequests, cloneRunRequest(request))
	if m.PreviewErr != nil {
		return m.PreviewErr
	}
	return m.Err
}

func (m *MockClient) QueuePipeline(_ context.Context, _ string, request RunRequest) (PipelineRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.QueueRequests = append(m.QueueRequests, cloneRunRequest(request))
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

func (m *MockClient) GetPipelineRun(_ context.Context, _ string, runID int) (PipelineRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
		PipelineID: request.PipelineID,
		Branch:     request.Branch,
		Parameters: make(map[string]string, len(request.Parameters)),
	}
	for key, value := range request.Parameters {
		clone.Parameters[key] = value
	}
	return clone
}
