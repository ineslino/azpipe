package azdo

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/google/uuid"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/build"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/core"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

type azdoClient struct {
	conn *azuredevops.Connection
}

// New returns a Client authenticated with the given PAT against orgURL.
// orgURL must be a full URL, e.g. "https://dev.azure.com/myorg".
func New(orgURL, pat string) Client {
	return &azdoClient{
		conn: azuredevops.NewPatConnection(orgURL, pat),
	}
}

func (c *azdoClient) ListProjects(ctx context.Context) ([]Project, error) {
	cc, err := core.NewClient(ctx, c.conn)
	if err != nil {
		return nil, fmt.Errorf("create core client: %w", err)
	}

	var projects []Project
	var token *int

	for {
		resp, err := cc.GetProjects(ctx, core.GetProjectsArgs{
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
		for _, p := range resp.Value {
			projects = append(projects, Project{
				ID:   uuidStr(p.Id),
				Name: derefStr(p.Name),
				URL:  derefStr(p.Url),
			})
		}
		if resp.ContinuationToken == "" {
			break
		}
		t, err := strconv.Atoi(resp.ContinuationToken)
		if err != nil {
			break
		}
		token = &t
	}
	return projects, nil
}

func (c *azdoClient) ListRepositories(ctx context.Context, project string) ([]Repository, error) {
	gc, err := git.NewClient(ctx, c.conn)
	if err != nil {
		return nil, fmt.Errorf("create git client: %w", err)
	}
	repos, err := gc.GetRepositories(ctx, git.GetRepositoriesArgs{Project: &project})
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	var result []Repository
	for _, r := range *repos {
		result = append(result, Repository{
			ID:            uuidStr(r.Id),
			Name:          derefStr(r.Name),
			DefaultBranch: derefStr(r.DefaultBranch),
			RemoteURL:     derefStr(r.RemoteUrl),
		})
	}
	return result, nil
}

func (c *azdoClient) ListPipelines(ctx context.Context, project string) ([]Pipeline, error) {
	bc, err := build.NewClient(ctx, c.conn)
	if err != nil {
		return nil, fmt.Errorf("create build client: %w", err)
	}
	top := 500
	incLatest := true
	defs, err := bc.GetDefinitions(ctx, build.GetDefinitionsArgs{
		Project:             &project,
		Top:                 &top,
		IncludeLatestBuilds: &incLatest,
	})
	if err != nil {
		return nil, fmt.Errorf("list pipelines: %w", err)
	}
	var result []Pipeline
	for _, d := range defs.Value {
		p := Pipeline{
			ID:     derefInt(d.Id),
			Name:   derefStr(d.Name),
			Folder: derefStr(d.Path),
		}
		// Repository is only available through the latest build reference.
		if d.LatestBuild != nil && d.LatestBuild.Repository != nil {
			p.RepoName = derefStr(d.LatestBuild.Repository.Name)
		}
		result = append(result, p)
	}
	return result, nil
}

func (c *azdoClient) GetPipelineRuns(ctx context.Context, project string, pipelineID int, limit int) ([]PipelineRun, error) {
	bc, err := build.NewClient(ctx, c.conn)
	if err != nil {
		return nil, fmt.Errorf("create build client: %w", err)
	}
	top := limit
	builds, err := bc.GetBuilds(ctx, build.GetBuildsArgs{
		Project:     &project,
		Definitions: &[]int{pipelineID},
		Top:         &top,
	})
	if err != nil {
		return nil, fmt.Errorf("get pipeline runs: %w", err)
	}
	var result []PipelineRun
	for _, b := range builds.Value {
		result = append(result, buildToRun(b))
	}
	return result, nil
}

func (c *azdoClient) GetActiveRun(ctx context.Context, project string, pipelineID int) (*PipelineRun, error) {
	bc, err := build.NewClient(ctx, c.conn)
	if err != nil {
		return nil, fmt.Errorf("create build client: %w", err)
	}
	top := 1
	status := build.BuildStatusValues.InProgress
	builds, err := bc.GetBuilds(ctx, build.GetBuildsArgs{
		Project:      &project,
		Definitions:  &[]int{pipelineID},
		StatusFilter: &status,
		Top:          &top,
	})
	if err != nil {
		return nil, fmt.Errorf("get active run: %w", err)
	}
	if len(builds.Value) == 0 {
		return nil, nil
	}
	run := buildToRun(builds.Value[0])
	return &run, nil
}

func (c *azdoClient) GetBuildTimeline(ctx context.Context, project string, buildID int) ([]StageResult, error) {
	bc, err := build.NewClient(ctx, c.conn)
	if err != nil {
		return nil, fmt.Errorf("create build client: %w", err)
	}
	tl, err := bc.GetBuildTimeline(ctx, build.GetBuildTimelineArgs{
		Project: &project,
		BuildId: &buildID,
	})
	if err != nil {
		return nil, fmt.Errorf("get build timeline: %w", err)
	}
	if tl == nil || tl.Records == nil {
		return nil, nil
	}
	var result []StageResult
	for _, r := range *tl.Records {
		if r.Name == nil || r.Type == nil {
			continue
		}
		sr := StageResult{
			Name:       derefStr(r.Name),
			RecordType: derefStr(r.Type),
			Order:      derefInt(r.Order),
		}
		if r.State != nil {
			sr.State = string(*r.State)
		}
		if r.Result != nil {
			sr.Result = string(*r.Result)
		}
		result = append(result, sr)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Order < result[j].Order
	})
	return result, nil
}

func (c *azdoClient) GetRepoPipelines(ctx context.Context, project string, repoName string) ([]Pipeline, error) {
	bc, err := build.NewClient(ctx, c.conn)
	if err != nil {
		return nil, fmt.Errorf("create build client: %w", err)
	}
	repoType := "TfsGit"
	defs, err := bc.GetDefinitions(ctx, build.GetDefinitionsArgs{
		Project:        &project,
		RepositoryId:   &repoName,
		RepositoryType: &repoType,
	})
	if err != nil {
		return nil, fmt.Errorf("get repo pipelines: %w", err)
	}
	var result []Pipeline
	for _, d := range defs.Value {
		result = append(result, Pipeline{
			ID:       derefInt(d.Id),
			Name:     derefStr(d.Name),
			Folder:   derefStr(d.Path),
			RepoName: repoName,
		})
	}
	return result, nil
}

// buildToRun converts a build.Build to a PipelineRun.
func buildToRun(b build.Build) PipelineRun {
	r := PipelineRun{
		ID:          derefInt(b.Id),
		BuildNumber: derefStr(b.BuildNumber),
		Branch:      derefStr(b.SourceBranch),
	}
	if b.Status != nil {
		r.State = string(*b.Status)
	}
	if b.Result != nil {
		r.Result = string(*b.Result)
	}
	if b.StartTime != nil {
		r.StartTime = b.StartTime.Time
	}
	if b.FinishTime != nil {
		r.FinishTime = b.FinishTime.Time
		if !r.StartTime.IsZero() {
			r.Duration = r.FinishTime.Sub(r.StartTime)
		}
	}
	return r
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

func uuidStr(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
