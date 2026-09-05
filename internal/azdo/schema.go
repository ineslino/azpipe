package azdo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/build"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"go.yaml.in/yaml/v3"
)

// Parameter schemas come from the root YAML at an exact Azure Repos commit.
type Parameter struct {
	Name         string     `yaml:"name"`
	DisplayName  string     `yaml:"displayName"`
	Type         string     `yaml:"type"`
	Default      *yaml.Node `yaml:"-"`
	DefaultValue string     `yaml:"-"`
	HasDefault   bool       `yaml:"-"`
	Values       []string   `yaml:"-"`
}
type ParameterSchema struct {
	Parameters        []Parameter
	Commit            string
	DefinitionVersion int
}
type SchemaProvider interface {
	GetPipelineSchema(context.Context, string, int, string) (ParameterSchema, error)
}

func ParseParameterSchema(content string) (ParameterSchema, error) {
	var document struct {
		Parameters yaml.Node `yaml:"parameters"`
	}
	if len(content) > 2<<20 {
		return ParameterSchema{}, fmt.Errorf("pipeline YAML excede 2 MiB")
	}
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		return ParameterSchema{}, err
	}
	s := ParameterSchema{}
	n := document.Parameters
	if n.Kind == 0 {
		return s, nil
	}
	if n.Kind != yaml.SequenceNode {
		return s, fmt.Errorf("parameters deve ser uma lista de declarações YAML")
	}
	seen := map[string]bool{}
	for _, entry := range n.Content {
		var p Parameter
		if err := entry.Decode(&p); err != nil {
			return s, err
		}
		if p.Name == "" || seen[p.Name] {
			return s, fmt.Errorf("nome de parâmetro vazio ou duplicado")
		}
		seen[p.Name] = true
		if p.Type == "" {
			p.Type = "string"
		}
		if p.DisplayName == "" {
			p.DisplayName = p.Name
		}
		for i := 0; i+1 < len(entry.Content); i += 2 {
			key, value := entry.Content[i].Value, entry.Content[i+1]
			if key == "default" {
				p.HasDefault = true
				p.Default = value
				p.DefaultValue = value.Value
			}
			if key == "values" {
				for _, option := range value.Content {
					p.Values = append(p.Values, option.Value)
				}
			}
		}
		s.Parameters = append(s.Parameters, p)
	}
	return s, nil
}

func (p Parameter) Editable() bool {
	return p.Type == "string" || p.Type == "boolean" || p.Type == "number"
}
func (p Parameter) Validate(value string) error {
	if !p.Editable() {
		return fmt.Errorf("%s: tipo %s não editável nesta TUI", p.Name, p.Type)
	}
	if p.Type == "boolean" && value != "true" && value != "false" {
		return fmt.Errorf("%s: escolha true ou false", p.Name)
	}
	if p.Type == "number" {
		number, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("%s: número inválido", p.Name)
		}
	}
	if len(p.Values) > 0 {
		for _, allowed := range p.Values {
			if value == allowed {
				return nil
			}
		}
		return fmt.Errorf("%s: valor fora das opções permitidas", p.Name)
	}
	return nil
}
func (s ParameterSchema) Validate(values map[string]string) error {
	known := map[string]bool{}
	for _, p := range s.Parameters {
		known[p.Name] = true
		value, sent := values[p.Name]
		if !sent {
			if !p.HasDefault {
				return fmt.Errorf("parâmetro obrigatório por preencher: %s", p.Name)
			}
			continue
		}
		if err := p.Validate(value); err != nil {
			return err
		}
	}
	for name := range values {
		if !known[name] {
			return fmt.Errorf("parâmetro não declarado: %s", name)
		}
	}
	return nil
}

func yamlPath(d *build.BuildDefinition) (string, error) {
	if d == nil || d.Repository == nil || derefStr(d.Repository.Type) != "TfsGit" {
		return "", fmt.Errorf("formulário requer pipeline YAML em Azure Repos Git")
	}
	data, err := json.Marshal(d.Process)
	if err != nil {
		return "", err
	}
	var process struct {
		YamlFilename string `json:"yamlFilename"`
	}
	if err = json.Unmarshal(data, &process); err != nil {
		return "", err
	}
	if process.YamlFilename == "" {
		return "", fmt.Errorf("definição sem caminho YAML")
	}
	return process.YamlFilename, nil
}

func (c *azdoClient) GetPipelineSchema(ctx context.Context, project string, id int, branch string) (ParameterSchema, error) {
	bc, err := build.NewClient(ctx, c.conn)
	if err != nil {
		return ParameterSchema{}, err
	}
	d, err := bc.GetDefinition(ctx, build.GetDefinitionArgs{Project: &project, DefinitionId: &id})
	if err != nil {
		return ParameterSchema{}, err
	}
	path, err := yamlPath(d)
	if err != nil {
		return ParameterSchema{}, err
	}
	gc, err := git.NewClient(ctx, c.conn)
	if err != nil {
		return ParameterSchema{}, err
	}
	ref := normalizeBranch(branch)
	filter := strings.TrimPrefix(ref, "refs/")
	refs, err := gc.GetRefs(ctx, git.GetRefsArgs{Project: &project, RepositoryId: d.Repository.Id, Filter: &filter})
	if err != nil {
		return ParameterSchema{}, err
	}
	commit := ""
	if refs != nil {
		for _, r := range refs.Value {
			if derefStr(r.Name) == ref {
				commit = derefStr(r.ObjectId)
			}
		}
	}
	if len(commit) != 40 {
		return ParameterSchema{}, fmt.Errorf("branch sem SHA resolvido")
	}
	include := true
	kind := git.GitVersionType("commit")
	item, err := gc.GetItem(ctx, git.GetItemArgs{Project: &project, RepositoryId: d.Repository.Id, Path: &path, IncludeContent: &include, VersionDescriptor: &git.GitVersionDescriptor{Version: &commit, VersionType: &kind}})
	if err != nil {
		return ParameterSchema{}, err
	}
	if item == nil || item.Content == nil {
		return ParameterSchema{}, fmt.Errorf("resposta sem conteúdo YAML")
	}
	s, err := ParseParameterSchema(*item.Content)
	s.Commit = commit
	s.DefinitionVersion = derefInt(d.Revision)
	return s, err
}

func (c *CommandClient) GetPipelineSchema(ctx context.Context, project string, id int, branch string) (ParameterSchema, error) {
	var d build.BuildDefinition
	_, err := c.invoke(ctx, "build", "definitions", project, "GET", []string{fmt.Sprintf("definitionId=%d", id)}, nil, nil, &d)
	if err != nil {
		return ParameterSchema{}, err
	}
	path, err := yamlPath(&d)
	if err != nil {
		return ParameterSchema{}, err
	}
	var refs struct {
		Value []git.GitRef `json:"value"`
	}
	ref := normalizeBranch(branch)
	_, err = c.invoke(ctx, "git", "refs", project, "GET", []string{"repositoryId=" + derefStr(d.Repository.Id)}, []string{"filter=" + strings.TrimPrefix(ref, "refs/")}, nil, &refs)
	if err != nil {
		return ParameterSchema{}, err
	}
	commit := ""
	for _, r := range refs.Value {
		if derefStr(r.Name) == ref {
			commit = derefStr(r.ObjectId)
		}
	}
	if len(commit) != 40 {
		return ParameterSchema{}, fmt.Errorf("branch sem SHA resolvido")
	}
	var item git.GitItem
	_, err = c.invoke(ctx, "git", "items", project, "GET", []string{"repositoryId=" + derefStr(d.Repository.Id)}, []string{"path=" + path, "includeContent=true", "versionDescriptor.versionType=commit", "versionDescriptor.version=" + commit}, nil, &item)
	if err != nil {
		return ParameterSchema{}, err
	}
	if item.Content == nil {
		return ParameterSchema{}, fmt.Errorf("resposta sem conteúdo YAML")
	}
	s, err := ParseParameterSchema(*item.Content)
	s.Commit = commit
	s.DefinitionVersion = derefInt(d.Revision)
	return s, err
}
