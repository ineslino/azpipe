package runner

import (
	"encoding/json"
	"fmt"
	"github.com/ineslino/azpipe/internal/azdo"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ProfileSelection struct {
	ID         int               `json:"id"`
	Mode       Mode              `json:"mode"`
	Branch     string            `json:"branch"`
	Parameters map[string]string `json:"parameters"`
}
type Profile struct {
	Version      int                `json:"version"`
	Name         string             `json:"name"`
	Organization string             `json:"organization"`
	Project      string             `json:"project"`
	Selections   []ProfileSelection `json:"selections"`
}

func DataDirectory(kind string) (string, error) {
	if kind != "profiles" && kind != "runs" {
		return "", fmt.Errorf("directório inválido")
	}
	if base := os.Getenv("AZPIPE_DATA_DIR"); base != "" {
		if !filepath.IsAbs(base) {
			return "", fmt.Errorf("AZPIPE_DATA_DIR deve ser absoluto")
		}
		return filepath.Join(base, kind), nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "azpipe", kind), nil
}

func SameContext(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(b, "/"))
}

func SameOrganization(a, b string) bool {
	normalize := func(value string) string {
		value = strings.TrimRight(strings.ToLower(value), "/")
		if !strings.Contains(value, "/") && !strings.Contains(value, ".") {
			value = "https://dev.azure.com/" + value
		}
		return value
	}
	return normalize(a) == normalize(b)
}

func SaveProfile(profile Profile) error {
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`).MatchString(profile.Name) {
		return fmt.Errorf("nome: 1–64 letras, números, hífen ou underscore")
	}
	if err := validateProfile(profile); err != nil {
		return err
	}
	dir, err := DataDirectory("profiles")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, profile.Name+".json")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("não foi possível criar perfil (nomes existentes não são substituídos): %w", err)
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	return f.Close()
}

func validateProfile(p Profile) error {
	if p.Version != 1 || p.Name == "" || p.Organization == "" || p.Project == "" || len(p.Selections) == 0 || len(p.Selections) > 500 {
		return fmt.Errorf("perfil incompleto ou versão não suportada")
	}
	seen := map[int]bool{}
	for _, s := range p.Selections {
		if s.ID <= 0 || seen[s.ID] || (s.Mode != ModeRun && s.Mode != ModePlan) || strings.TrimSpace(s.Branch) == "" {
			return fmt.Errorf("selecção inválida no perfil")
		}
		seen[s.ID] = true
	}
	return nil
}

func readLocalJSON(path string, target any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 4<<20 {
		return fmt.Errorf("ficheiro inválido ou demasiado grande")
	}
	d := json.NewDecoder(f)
	if err = d.Decode(target); err != nil {
		return err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("JSON com conteúdo adicional")
	}
	return nil
}

func ListProfiles(organization, project string) ([]Profile, error) {
	dir, err := DataDirectory("profiles")
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	result := []Profile{}
	for _, path := range paths {
		var p Profile
		if err = readLocalJSON(path, &p); err != nil {
			return nil, fmt.Errorf("perfil %s: %w", filepath.Base(path), err)
		}
		if err = validateProfile(p); err != nil {
			return nil, err
		}
		if SameOrganization(p.Organization, organization) && SameContext(p.Project, project) {
			result = append(result, p)
		}
	}
	return result, nil
}

func (p Profile) Resolve(organization, project string, pipelines []azdo.Pipeline) ([]Selection, error) {
	if err := validateProfile(p); err != nil {
		return nil, err
	}
	if !SameOrganization(p.Organization, organization) || !SameContext(p.Project, project) {
		return nil, fmt.Errorf("perfil pertence a outro contexto")
	}
	index := map[int]azdo.Pipeline{}
	for _, pipeline := range pipelines {
		index[pipeline.ID] = pipeline
	}
	result := make([]Selection, 0, len(p.Selections))
	for _, saved := range p.Selections {
		pipeline, ok := index[saved.ID]
		if !ok {
			return nil, fmt.Errorf("pipeline %d já não está no catálogo", saved.ID)
		}
		if saved.Mode == ModePlan && pipeline.PlanContract == nil {
			return nil, fmt.Errorf("pipeline %d sem contrato PLAN actual", saved.ID)
		}
		params := map[string]string{}
		for k, v := range saved.Parameters {
			params[k] = v
		}
		result = append(result, Selection{Pipeline: pipeline, Mode: saved.Mode, Branch: saved.Branch, Inputs: params})
	}
	return result, nil
}
