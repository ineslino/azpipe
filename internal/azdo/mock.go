package azdo

import "context"

// MockClient is a test double for Client. Set the exported fields before use.
type MockClient struct {
	Projects   []Project
	Repos      []Repository
	Pipelines  []Pipeline
	Runs       []PipelineRun
	ActiveRun  *PipelineRun
	Timeline   []StageResult
	Err        error
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
