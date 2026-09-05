package azdo

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/build"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/pipelines"
)

// CommandClient keeps credentials inside the configured azdo-as process.
type CommandClient struct {
	Executable, Profile, ExpectedIdentity, Organization string
	Contracts                                           []PlanContract
	output                                              func(context.Context, ...string) ([]byte, error)
}

func (c *CommandClient) call(ctx context.Context, args ...string) ([]byte, error) {
	if c.output != nil {
		return c.output(ctx, args...)
	}
	return exec.CommandContext(ctx, c.Executable, args...).Output()
}

func (c *CommandClient) VerifyIdentity(ctx context.Context) error {
	if c.Executable == "" || c.ExpectedIdentity == "" {
		return fmt.Errorf("azdo-as requires executable and expected identity")
	}
	data, err := c.call(ctx, c.Profile, "whoami")
	if err != nil {
		return fmt.Errorf("azdo-as identity unavailable: renew credentials using your approved login process")
	}
	var identity struct {
		AuthenticatedUser struct {
			Properties map[string]struct {
				Value string `json:"$value"`
			} `json:"properties"`
		} `json:"authenticatedUser"`
	}
	if err := json.Unmarshal(data, &identity); err != nil {
		return fmt.Errorf("azdo-as returned invalid identity")
	}
	if !strings.EqualFold(identity.AuthenticatedUser.Properties["Account"].Value, c.ExpectedIdentity) {
		return fmt.Errorf("azdo-as identity differs from expected identity")
	}
	return nil
}

func (c *CommandClient) invoke(ctx context.Context, area, resource, project, method string, route, query []string, body any, out any) (string, error) {
	if err := c.VerifyIdentity(ctx); err != nil {
		return "", err
	}
	args := []string{c.Profile, "devops", "invoke", "--organization", c.Organization, "--detect", "false", "--area", area, "--resource", resource, "--api-version", "7.1", "--http-method", method, "--output", "json", "--only-show-errors"}
	if project != "" {
		route = append(route, "project="+project)
	}
	if len(route) > 0 {
		args = append(args, "--route-parameters")
		args = append(args, route...)
	}
	if len(query) > 0 {
		args = append(args, "--query-parameters")
		args = append(args, query...)
	}
	if body != nil {
		f, err := os.CreateTemp("", "azpipe-request-*.json")
		if err != nil {
			return "", err
		}
		defer os.Remove(f.Name())
		if err := json.NewEncoder(f).Encode(body); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		args = append(args, "--in-file", f.Name(), "--encoding", "utf-8")
	}
	data, err := c.call(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("azdo-as %s/%s failed; check credentials and permissions; a POST failure may have been accepted remotely", area, resource)
	}
	var envelope struct {
		Token string `json:"continuation_token"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return "", err
	}
	return envelope.Token, nil
}

func (c *CommandClient) ListPipelines(ctx context.Context, project string) ([]Pipeline, error) {
	var result []Pipeline
	token := ""
	seen := map[string]bool{}
	for {
		var page struct {
			Value []build.BuildDefinition `json:"value"`
		}
		query := []string{"$top=500", "includeAllProperties=true"}
		if token != "" {
			query = append(query, "continuationToken="+token)
		}
		next, err := c.invoke(ctx, "build", "definitions", project, "GET", nil, query, nil, &page)
		if err != nil {
			return nil, err
		}
		for _, d := range page.Value {
			p := Pipeline{ID: derefInt(d.Id), Name: derefStr(d.Name), Folder: derefStr(d.Path)}
			if d.Repository != nil {
				p.RepoName = derefStr(d.Repository.Name)
			}
			if d.Tags != nil {
				p.Tags = *d.Tags
			}
			for _, contract := range c.Contracts {
				if contract.PipelineID == p.ID && contract.Project == project && strings.EqualFold(strings.TrimRight(contract.Organization, "/"), strings.TrimRight(c.Organization, "/")) {
					copy := contract
					p.PlanContract = &copy
				}
			}
			result = append(result, p)
		}
		if next == "" {
			return result, nil
		}
		if seen[next] {
			return nil, fmt.Errorf("repeated continuation token")
		}
		seen[next] = true
		token = next
	}
}

func (c *CommandClient) ListProjects(ctx context.Context) ([]Project, error) {
	var out struct {
		Value []Project `json:"value"`
	}
	_, err := c.invoke(ctx, "core", "projects", "", "GET", nil, nil, nil, &out)
	return out.Value, err
}
func (c *CommandClient) ListRepositories(ctx context.Context, project string) ([]Repository, error) {
	var out struct {
		Value []Repository `json:"value"`
	}
	_, err := c.invoke(ctx, "git", "repositories", project, "GET", nil, nil, nil, &out)
	return out.Value, err
}
func (c *CommandClient) GetRepoPipelines(ctx context.Context, project, name string) ([]Pipeline, error) {
	all, err := c.ListPipelines(ctx, project)
	var result []Pipeline
	for _, p := range all {
		if p.RepoName == name {
			result = append(result, p)
		}
	}
	return result, err
}
func (c *CommandClient) GetPipelineRuns(ctx context.Context, project string, id, limit int) ([]PipelineRun, error) {
	var out struct {
		Value []build.Build `json:"value"`
	}
	_, err := c.invoke(ctx, "build", "builds", project, "GET", nil, []string{"definitions=" + strconv.Itoa(id), "$top=" + strconv.Itoa(limit)}, nil, &out)
	var result []PipelineRun
	for _, b := range out.Value {
		r := buildToRun(b)
		r.WebURL = buildWebURL(b.Links)
		result = append(result, r)
	}
	return result, err
}
func (c *CommandClient) GetActiveRun(ctx context.Context, project string, id int) (*PipelineRun, error) {
	runs, err := c.GetPipelineRuns(ctx, project, id, 1)
	if err != nil {
		return nil, err
	}
	if len(runs) > 0 && runs[0].State != "completed" {
		return &runs[0], nil
	}
	return nil, nil
}
func (c *CommandClient) GetBuildTimeline(ctx context.Context, project string, id int) ([]StageResult, error) {
	var out struct {
		Records []struct {
			Name   string
			Type   string
			State  string
			Result string
			Order  int
		} `json:"records"`
	}
	_, err := c.invoke(ctx, "build", "timeline", project, "GET", []string{"buildId=" + strconv.Itoa(id)}, nil, nil, &out)
	var result []StageResult
	for _, r := range out.Records {
		result = append(result, StageResult{Name: r.Name, RecordType: r.Type, State: r.State, Result: r.Result, Order: r.Order})
	}
	return result, err
}
func (c *CommandClient) GetPipelineRun(ctx context.Context, project string, id int) (PipelineRun, error) {
	var out build.Build
	_, err := c.invoke(ctx, "build", "builds", project, "GET", []string{"buildId=" + strconv.Itoa(id)}, nil, nil, &out)
	r := buildToRun(out)
	r.WebURL = buildWebURL(out.Links)
	return r, err
}
func (c *CommandClient) hash(ctx context.Context, project string, r RunRequest) (string, error) {
	var out pipelines.PreviewRun
	_, err := c.invoke(ctx, "pipelines", "preview", project, "POST", []string{"pipelineId=" + strconv.Itoa(r.PipelineID)}, []string{"pipelineVersion=" + strconv.Itoa(r.DefinitionVersion)}, runParameters(r), &out)
	if err != nil {
		return "", err
	}
	if out.FinalYaml == nil {
		return "", fmt.Errorf("preview without finalYaml")
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(*out.FinalYaml))), nil
}
func (c *CommandClient) PreviewPipeline(ctx context.Context, project string, r RunRequest) error {
	h, err := c.hash(ctx, project, r)
	if err != nil {
		return err
	}
	if r.PreviewHash != "" && h != r.PreviewHash {
		return fmt.Errorf("expanded YAML changed; repeat review")
	}
	return nil
}
func (c *CommandClient) QueuePipeline(ctx context.Context, project string, r RunRequest) (PipelineRun, error) {
	if err := c.PreviewPipeline(ctx, project, r); err != nil {
		return PipelineRun{}, err
	}
	var out pipelines.Run
	_, err := c.invoke(ctx, "pipelines", "runs", project, "POST", []string{"pipelineId=" + strconv.Itoa(r.PipelineID)}, []string{"pipelineVersion=" + strconv.Itoa(r.DefinitionVersion)}, runParameters(r), &out)
	if err != nil {
		return PipelineRun{}, err
	}
	if out.Id == nil {
		return PipelineRun{}, fmt.Errorf("submission uncertain: no run ID")
	}
	run := pipelineRunToRun(out)
	if run.WebURL == "" {
		run.WebURL = fmt.Sprintf("%s/%s/_build/results?buildId=%d", c.Organization, project, run.ID)
	}
	return run, nil
}
func (c *CommandClient) PrepareRun(ctx context.Context, project string, r RunRequest) (RunRequest, error) {
	var d build.BuildDefinition
	_, err := c.invoke(ctx, "build", "definitions", project, "GET", []string{"definitionId=" + strconv.Itoa(r.PipelineID)}, nil, nil, &d)
	if err != nil {
		return r, err
	}
	if d.Repository == nil || derefStr(d.Repository.Type) != "TfsGit" || d.Revision == nil {
		return r, fmt.Errorf("pinning requires Azure Repos Git")
	}
	var refs struct {
		Value []git.GitRef `json:"value"`
	}
	ref := normalizeBranch(r.Branch)
	_, err = c.invoke(ctx, "git", "refs", project, "GET", []string{"repositoryId=" + derefStr(d.Repository.Id)}, []string{"filter=" + strings.TrimPrefix(ref, "refs/")}, nil, &refs)
	if err != nil {
		return r, err
	}
	for _, v := range refs.Value {
		if derefStr(v.Name) == ref {
			r.Commit = derefStr(v.ObjectId)
		}
	}
	if len(r.Commit) != 40 {
		return r, fmt.Errorf("branch SHA unresolved")
	}
	r.DefinitionVersion = *d.Revision
	for _, contract := range c.Contracts {
		if contract.Project == project && contract.PipelineID == r.PipelineID && strings.EqualFold(strings.TrimRight(contract.Organization, "/"), strings.TrimRight(c.Organization, "/")) {
			if contract.Commit != r.Commit || contract.DefinitionVersion != r.DefinitionVersion {
				return r, fmt.Errorf("RUN/PLAN contract is stale")
			}
		}
	}
	r.PreviewHash, err = c.hash(ctx, project, r)
	return r, err
}

var _ Client = (*CommandClient)(nil)
