package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ineslino/azpipe/internal/azdo"
	"github.com/ineslino/azpipe/internal/localfile"
)

type JournalRecord struct {
	PipelineID   int              `json:"pipelineId"`
	Project      string           `json:"project,omitempty"`
	PipelineName string           `json:"pipelineName,omitempty"`
	Run          azdo.PipelineRun `json:"run"`
	Error        string           `json:"error,omitempty"`
}

type Journal struct {
	Organization string          `json:"organization"`
	Project      string          `json:"project"`
	Runs         []JournalRecord `json:"runs"`
	UpdatedAt    time.Time       `json:"-"`
	path         string
	mu           sync.Mutex
}

func NewJournal(organization, project string, reviews []Review) (*Journal, error) {
	if err := ValidateBatchSize(len(reviews)); err != nil {
		return nil, err
	}
	dir, err := DataDirectory("runs")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := localfile.Protect(dir); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, "batch-*.json")
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return nil, err
	}
	j := &Journal{Organization: organization, Project: project, path: f.Name(), Runs: make([]JournalRecord, len(reviews))}
	for i, r := range reviews {
		recordProject := r.Selection.Pipeline.Project
		if recordProject == "" {
			recordProject = project
		}
		j.Runs[i] = JournalRecord{PipelineID: r.Selection.ID(), Project: recordProject, PipelineName: r.Selection.Pipeline.Name, Error: "submissão incerta: verificar Azure DevOps antes de repetir"}
	}
	if err := j.save(); err != nil {
		os.Remove(j.path)
		return nil, err
	}
	return j, nil
}

func LoadJournal(path, organization, project string) (*Journal, error) {
	j := &Journal{path: path}
	if err := readLocalJSON(path, j); err != nil {
		return nil, err
	}
	if !SameOrganization(j.Organization, organization) || !SameContext(j.Project, project) {
		return nil, fmt.Errorf("lote pertence a outro contexto")
	}
	if ValidateBatchSize(len(j.Runs)) != nil {
		return nil, fmt.Errorf("lote vazio ou demasiado grande")
	}
	seen := map[int]bool{}
	for _, r := range j.Runs {
		if r.PipelineID <= 0 || r.Run.ID < 0 || (r.Run.ID > 0 && seen[r.Run.ID]) {
			return nil, fmt.Errorf("IDs inválidos no lote")
		}
		seen[r.Run.ID] = true
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	j.UpdatedAt = info.ModTime()
	return j, nil
}

func ListJournals(organization, project string) ([]*Journal, error) {
	dir, err := DataDirectory("runs")
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	result := []*Journal{}
	for _, path := range paths {
		var header struct {
			Organization string
			Project      string
		}
		if err = readLocalJSON(path, &header); err != nil {
			return nil, err
		}
		if !SameOrganization(header.Organization, organization) || !SameContext(header.Project, project) {
			continue
		}
		j, err := LoadJournal(path, organization, project)
		if err != nil {
			return nil, err
		}
		result = append(result, j)
	}
	sort.Slice(result, func(i, k int) bool {
		if result[i].UpdatedAt.Equal(result[k].UpdatedAt) {
			return result[i].path > result[k].path
		}
		return result[i].UpdatedAt.After(result[k].UpdatedAt)
	})
	return result, nil
}

func (j *Journal) Results() []RunResult {
	results := make([]RunResult, len(j.Runs))
	for i, r := range j.Runs {
		name := r.PipelineName
		if name == "" {
			name = fmt.Sprintf("pipeline %d", r.PipelineID)
		}
		project := r.Project
		if project == "" && j.Project != AllProjects {
			project = j.Project
		}
		results[i] = RunResult{Review: Review{Selection: Selection{Pipeline: azdo.Pipeline{ID: r.PipelineID, Project: project, Name: name}}}, Run: r.Run}
		if r.Run.ID == 0 {
			results[i].Err = fmt.Errorf("submissão incerta: verificar Azure DevOps; retoma não volta a lançar")
		}
	}
	return results
}

func (j *Journal) UpdateRuns(results []RunResult) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(results) != len(j.Runs) {
		return fmt.Errorf("lote alterado durante monitorização")
	}
	for i, r := range results {
		if r.Run.ID != j.Runs[i].Run.ID {
			return fmt.Errorf("ID alterado durante monitorização")
		}
	}
	for i, r := range results {
		j.Runs[i].Run = r.Run
		if r.Err != nil {
			j.Runs[i].Error = r.Err.Error()
		} else if r.Run.ID > 0 {
			j.Runs[i].Error = ""
		}
	}
	return j.save()
}

func (j *Journal) Path() string { return j.path }

func (j *Journal) Record(index int, result RunResult) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Runs[index].Run = result.Run
	if result.Err == nil && result.Run.ID > 0 {
		j.Runs[index].Error = ""
	} else {
		j.Runs[index].Error = fmt.Sprintf("submissão incerta: %v; verificar antes de repetir", result.Err)
	}
	return j.save()
}

func (j *Journal) save() error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return localfile.WriteAtomic(j.path, append(data, '\n'))
}
