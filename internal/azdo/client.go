package azdo

import (
	"context"
	"strings"
	"time"
)

// Client is the interface for all Azure DevOps operations used by azpipe.
// Keeping API calls behind this interface allows full mock replacement in tests.
type Client interface {
	ListProjects(ctx context.Context) ([]Project, error)
	ListRepositories(ctx context.Context, project string) ([]Repository, error)
	ListPipelines(ctx context.Context, project string) ([]Pipeline, error)
	GetPipelineRuns(ctx context.Context, project string, pipelineID int, limit int) ([]PipelineRun, error)
	GetActiveRun(ctx context.Context, project string, pipelineID int) (*PipelineRun, error)
	GetBuildTimeline(ctx context.Context, project string, buildID int) ([]StageResult, error)
	GetRepoPipelines(ctx context.Context, project string, repoName string) ([]Pipeline, error)
	PreviewPipeline(ctx context.Context, project string, request RunRequest) error
	QueuePipeline(ctx context.Context, project string, request RunRequest) (PipelineRun, error)
	GetPipelineRun(ctx context.Context, project string, runID int) (PipelineRun, error)
}

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Repository struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"defaultBranch"`
	RemoteURL     string `json:"remoteUrl"`
}

type Pipeline struct {
	ID              int           `json:"id"`
	Name            string        `json:"name"`
	Folder          string        `json:"folder"`
	RepoName        string        `json:"repoName"`
	Tags            []string      `json:"tags"`
	MetadataWarning string        `json:"metadataWarning,omitempty"`
	PlanContract    *PlanContract `json:"planContract,omitempty"`
}

type PlanContract struct {
	Organization      string `json:"organization"`
	Project           string `json:"project"`
	PipelineID        int    `json:"pipelineId"`
	DefinitionVersion int    `json:"definitionVersion"`
	Commit            string `json:"commit"`
	Parameter         string `json:"parameter"`
	Type              string `json:"type"`
	PlanValue         string `json:"planValue"`
	RunValue          string `json:"runValue"`
	Evidence          string `json:"evidence"`
}

// Type returns the pipeline's top-level folder, or root for ungrouped pipelines.
func (p Pipeline) Type() string {
	for _, segment := range strings.Split(strings.ReplaceAll(p.Folder, "\\", "/"), "/") {
		if segment != "" {
			return segment
		}
	}
	return "root"
}

type RunRequest struct {
	PipelineID        int
	Branch            string
	Parameters        map[string]string
	Commit            string
	DefinitionVersion int
	PreviewHash       string
}

type PipelineRun struct {
	ID          int           `json:"id"`
	BuildNumber string        `json:"buildNumber"`
	State       string        `json:"state"`
	Result      string        `json:"result"`
	StartTime   time.Time     `json:"startTime"`
	FinishTime  time.Time     `json:"finishTime"`
	Duration    time.Duration `json:"durationMs"`
	Branch      string        `json:"branch"`
	WebURL      string        `json:"webUrl"`
}

type StageResult struct {
	Name       string `json:"name"`
	RecordType string `json:"recordType"`
	State      string `json:"state"`
	Result     string `json:"result"`
	Order      int    `json:"order"`
}
