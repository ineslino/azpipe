package azdo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// BranchClient is optional so pipeline-only clients need no destructive capability.
type BranchClient interface {
	ListBranches(context.Context, string, string) ([]Branch, error)
	ReviewBranch(context.Context, string, string, Branch) (Branch, error)
	DeleteBranch(context.Context, string, string, Branch) error
}

type Branch struct {
	repositoryID string
	Name         string `json:"name"`
	ObjectID     string `json:"objectId"`
	Creator      struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		UniqueName  string `json:"uniqueName"`
	} `json:"creator"`
	IsLocked bool   `json:"isLocked"`
	Blocked  string `json:"blocked,omitempty"`
}

func (b Branch) Matches(name, creator string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	creator = strings.ToLower(strings.TrimSpace(creator))
	identity := strings.ToLower(strings.Join([]string{b.Creator.ID, b.Creator.DisplayName, b.Creator.UniqueName}, " "))
	return strings.Contains(strings.ToLower(b.Name), name) && strings.Contains(identity, creator)
}

type branchRequest func(context.Context, string, string, string, string, url.Values, any, any) (string, error)

func (c *azdoClient) branchRequest(ctx context.Context, project, repo, resource, method string, query url.Values, body, out any) (string, error) {
	endpoint := strings.TrimRight(c.conn.BaseUrl, "/") + "/" + url.PathEscape(project) + "/_apis/git/"
	if resource == "policyConfigurations" {
		endpoint += "policy/configurations"
	} else {
		endpoint += "repositories/" + url.PathEscape(repo)
		if resource != "repositories" {
			endpoint += "/" + resource
		}
	}
	if query == nil {
		query = url.Values{}
	}
	query.Set("api-version", "7.1")
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return "", err
		}
	}
	client := c.conn.GetClientByUrl(c.conn.BaseUrl)
	req, err := client.CreateRequestMessage(ctx, method, endpoint+"?"+query.Encode(), "7.1", bytes.NewReader(data), "application/json", "application/json", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.SendRequest(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return "", fmt.Errorf("consulta de branches falhou (%s %s)", method, resource)
	}
	if resp == nil {
		return "", fmt.Errorf("resposta de branches ausente")
	}
	if err = json.NewDecoder(resp.Body).Decode(out); err != nil {
		return "", fmt.Errorf("resposta de branches inválida")
	}
	return resp.Header.Get("x-ms-continuationtoken"), nil
}

func (c *CommandClient) branchRequest(ctx context.Context, project, repo, resource, method string, query url.Values, body, out any) (string, error) {
	route := []string{}
	if resource != "policyConfigurations" {
		route = append(route, "repositoryId="+repo)
	}
	var params []string
	for key, values := range query {
		for _, value := range values {
			params = append(params, key+"="+value)
		}
	}
	return c.invoke(ctx, "git", resource, project, method, route, params, body, out)
}

func listBranches(ctx context.Context, request branchRequest, project, repo string) ([]Branch, error) {
	query := url.Values{"filter": {"heads/"}, "$top": {"1000"}}
	var result []Branch
	seen := map[string]bool{}
	for {
		var page struct {
			Value []Branch `json:"value"`
		}
		token, err := request(ctx, project, repo, "refs", "GET", query, nil, &page)
		if err != nil {
			return nil, err
		}
		if page.Value == nil {
			return nil, fmt.Errorf("lista de branches ausente na resposta")
		}
		for _, b := range page.Value {
			if strings.HasPrefix(b.Name, "refs/heads/") {
				result = append(result, b)
			}
		}
		if token == "" {
			break
		}
		if seen[token] {
			return nil, fmt.Errorf("paginação de branches repetida")
		}
		seen[token] = true
		query.Set("continuationToken", token)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func findBranch(ctx context.Context, request branchRequest, project, repo, name string) (Branch, error) {
	query := url.Values{"filter": {strings.TrimPrefix(name, "refs/")}, "$top": {"1000"}}
	seen := map[string]bool{}
	for {
		var page struct {
			Value []Branch `json:"value"`
		}
		token, err := request(ctx, project, repo, "refs", "GET", query, nil, &page)
		if err != nil {
			return Branch{}, err
		}
		if page.Value == nil {
			return Branch{}, fmt.Errorf("lista de branches ausente na resposta")
		}
		for _, branch := range page.Value {
			if branch.Name == name {
				return branch, nil
			}
		}
		if token == "" {
			return Branch{}, fmt.Errorf("branch já não existe")
		}
		if seen[token] {
			return Branch{}, fmt.Errorf("paginação de branches repetida")
		}
		seen[token] = true
		query.Set("continuationToken", token)
	}
}

var branchSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

const zeroSHA = "0000000000000000000000000000000000000000"

func reviewBranch(ctx context.Context, request branchRequest, project, repo string, selected Branch) (Branch, error) {
	if project == "" || repo == "" || !strings.HasPrefix(selected.Name, "refs/heads/") || !branchSHA.MatchString(selected.ObjectID) || selected.ObjectID == zeroSHA {
		return selected, fmt.Errorf("branch, contexto ou SHA inválido")
	}
	selected.Blocked = ""
	var repository Repository
	if _, err := request(ctx, project, repo, "repositories", "GET", nil, nil, &repository); err != nil {
		return selected, err
	}
	if repository.ID == "" || repository.DefaultBranch == "" {
		return selected, fmt.Errorf("não foi possível confirmar o repositório e a branch default")
	}
	selected.repositoryID = repository.ID
	current, err := findBranch(ctx, request, project, repository.ID, selected.Name)
	if err != nil {
		return selected, err
	}
	if current.ObjectID != selected.ObjectID {
		return selected, fmt.Errorf("branch alterada desde a selecção; actualiza e revê novamente")
	}
	selected.IsLocked = current.IsLocked
	selected.Creator = current.Creator
	name := strings.ToLower(strings.TrimPrefix(selected.Name, "refs/heads/"))
	if strings.EqualFold(selected.Name, repository.DefaultBranch) || name == "main" || name == "master" || name == "develop" {
		selected.Blocked = "branch principal protegida"
		return selected, nil
	}
	if selected.IsLocked {
		selected.Blocked = "branch bloqueada"
		return selected, nil
	}
	query := url.Values{"repositoryId": {repository.ID}, "refName": {selected.Name}, "$top": {"1000"}}
	seen := map[string]bool{}
	for {
		var policies struct {
			Value []struct {
				Enabled *bool `json:"isEnabled"`
			} `json:"value"`
		}
		token, err := request(ctx, project, repository.ID, "policyConfigurations", "GET", query, nil, &policies)
		if err != nil {
			return selected, fmt.Errorf("não foi possível verificar políticas: %w", err)
		}
		if policies.Value == nil {
			return selected, fmt.Errorf("lista de políticas ausente na resposta")
		}
		for _, p := range policies.Value {
			if p.Enabled == nil {
				return selected, fmt.Errorf("estado da política ausente na resposta")
			}
			if *p.Enabled {
				selected.Blocked = "política de branch aplicável"
				return selected, nil
			}
		}
		if token == "" {
			break
		}
		if seen[token] {
			return selected, fmt.Errorf("paginação de políticas repetida")
		}
		seen[token] = true
		query.Set("continuationToken", token)
	}
	// Check both source and target: deleting a target can disrupt an active PR too.
	for _, field := range []string{"sourceRefName", "targetRefName"} {
		var prs struct {
			Value []json.RawMessage `json:"value"`
		}
		if _, err := request(ctx, project, repository.ID, "pullRequests", "GET", url.Values{"searchCriteria.status": {"active"}, "searchCriteria." + field: {selected.Name}, "$top": {"1"}}, nil, &prs); err != nil {
			return selected, fmt.Errorf("não foi possível verificar PRs: %w", err)
		}
		if prs.Value == nil {
			return selected, fmt.Errorf("lista de PRs ausente na resposta")
		}
		if len(prs.Value) > 0 {
			selected.Blocked = "PR activo associado"
			return selected, nil
		}
	}
	return selected, nil
}

func deleteBranch(ctx context.Context, request branchRequest, project, repo string, selected Branch) error {
	reviewed, err := reviewBranch(ctx, request, project, repo, selected)
	if err != nil {
		return err
	}
	if reviewed.Blocked != "" {
		return fmt.Errorf("eliminação bloqueada: %s", reviewed.Blocked)
	}
	body := []map[string]string{{"name": reviewed.Name, "oldObjectId": reviewed.ObjectID, "newObjectId": zeroSHA}}
	var result struct {
		Value []struct {
			Name    string `json:"name"`
			Success bool   `json:"success"`
			Status  string `json:"updateStatus"`
		} `json:"value"`
	}
	if _, err = request(ctx, project, reviewed.repositoryID, "refs", "POST", nil, body, &result); err != nil {
		return fmt.Errorf("resultado incerto; confirma no Azure DevOps antes de repetir: %w", err)
	}
	if len(result.Value) != 1 || result.Value[0].Name != reviewed.Name {
		return fmt.Errorf("resposta inesperada; confirma no Azure DevOps antes de repetir")
	}
	if !result.Value[0].Success || result.Value[0].Status != "succeeded" {
		return fmt.Errorf("eliminação não confirmada: %s", result.Value[0].Status)
	}
	return nil
}

func (c *azdoClient) ListBranches(ctx context.Context, p, r string) ([]Branch, error) {
	return listBranches(ctx, c.branchRequest, p, r)
}
func (c *CommandClient) ListBranches(ctx context.Context, p, r string) ([]Branch, error) {
	return listBranches(ctx, c.branchRequest, p, r)
}
func (c *azdoClient) ReviewBranch(ctx context.Context, p, r string, b Branch) (Branch, error) {
	return reviewBranch(ctx, c.branchRequest, p, r, b)
}
func (c *CommandClient) ReviewBranch(ctx context.Context, p, r string, b Branch) (Branch, error) {
	return reviewBranch(ctx, c.branchRequest, p, r, b)
}
func (c *azdoClient) DeleteBranch(ctx context.Context, p, r string, b Branch) error {
	return deleteBranch(ctx, c.branchRequest, p, r, b)
}
func (c *CommandClient) DeleteBranch(ctx context.Context, p, r string, b Branch) error {
	return deleteBranch(ctx, c.branchRequest, p, r, b)
}
