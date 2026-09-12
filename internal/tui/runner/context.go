package runner

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
	domainrunner "github.com/ineslino/azpipe/internal/runner"
)

// ClientFactory creates an authenticated client after the operator submits an organization.
type ClientFactory func(organization string) (azdo.Client, error)

// ContextDefaults pre-fills non-secret context values from flags or configuration.
type ContextDefaults struct {
	Organization string
	Project      string
}

type contextSubmitMsg struct {
	organization string
	project      string
}

type projectsLoadedMsg struct {
	token        operationToken
	organization string
	client       azdo.Client
	projects     []azdo.Project
	err          error
}

type contextLoadedMsg struct {
	token        operationToken
	organization string
	client       azdo.Client
	project      string
	pipelines    []azdo.Pipeline
	err          error
}

const (
	contextOrganizationFocus = iota
	contextProjectFocus
	allProjectsLabel        = "Todos os projectos"
	allProjectsCapacity     = 10
	pipelineProjectParallel = 4
)

type contextModel struct {
	width, height  int
	errorScroll    int
	organization   textinput.Model
	project        textinput.Model // retained for compatibility with the non-interactive bootstrap contract
	projects       []azdo.Project
	projectCursor  int
	projectDefault string
	focus          int
	loading        bool
	err            string
}

func newContextModel(defaults ContextDefaults) contextModel {
	organization := textinput.New()
	organization.Prompt = "Organização: "
	organization.CharLimit = 256
	organization.Width = 48
	organization.PromptStyle = keyStyle
	organization.SetValue(defaults.Organization)
	organization.Focus()
	project := textinput.New()
	project.SetValue(defaults.Project)
	project.Blur()

	return contextModel{
		organization:   organization,
		project:        project,
		projectDefault: strings.TrimSpace(defaults.Project),
	}
}

func (m contextModel) update(msg tea.Msg) (contextModel, tea.Cmd) {
	key, isKey := msg.(tea.KeyMsg)
	if isKey {
		if m.err != "" {
			if key.String() == "pgdown" {
				m.errorScroll++
				return m, nil
			}
			if key.String() == "pgup" {
				m.errorScroll = max(0, m.errorScroll-1)
				return m, nil
			}
		}
		if key.String() == "ctrl+c" || key.String() == "ctrl+d" {
			return m, tea.Quit
		}
		if key.Type == tea.KeyEsc {
			if m.focus == contextProjectFocus && len(m.projects) > 0 && !m.loading {
				m.projects = nil
				m.projectCursor = 0
				m.err = ""
				m.setFocus(contextOrganizationFocus)
				return m, nil
			}
			return m, tea.Quit
		}
		if m.loading {
			return m, nil
		}
		if m.focus == contextProjectFocus && len(m.projects) > 0 {
			switch key.String() {
			case "up", "k", "shift+tab":
				m.projectCursor = max(0, m.projectCursor-1)
				return m, nil
			case "down", "j", "tab":
				m.projectCursor = min(len(m.projects), m.projectCursor+1)
				return m, nil
			case "enter":
				organization := strings.TrimSpace(m.organization.Value())
				return m, func() tea.Msg {
					return contextSubmitMsg{organization: organization, project: m.selectedProject()}
				}
			}
		}
		switch key.String() {
		case "tab", "down":
			if len(m.projects) > 0 {
				m.setFocus(contextProjectFocus)
			}
			return m, nil
		case "shift+tab", "up":
			m.setFocus(contextOrganizationFocus)
			return m, nil
		case "enter":
			organization := strings.TrimSpace(m.organization.Value())
			if organization == "" {
				m.err = "A organização é obrigatória."
				return m, nil
			}
			m.err = ""
			m.loading = true
			return m, func() tea.Msg {
				return contextSubmitMsg{organization: organization}
			}
		}
	}

	var cmd tea.Cmd
	if m.focus == contextOrganizationFocus {
		m.organization, cmd = m.organization.Update(msg)
	}
	return m, cmd
}

func (m *contextModel) setFocus(focus int) {
	m.focus = focus
	if focus == contextOrganizationFocus {
		m.project.Blur()
		m.organization.Focus()
		return
	}
	m.organization.Blur()
	m.project.Blur()
}

func (m *contextModel) setProjects(projects []azdo.Project) {
	m.projects = append([]azdo.Project(nil), projects...)
	sort.SliceStable(m.projects, func(i, j int) bool {
		return strings.ToLower(m.projects[i].Name) < strings.ToLower(m.projects[j].Name)
	})
	m.projectCursor = 0
	defaultProject := strings.TrimSpace(m.projectDefault)
	if defaultProject == "" || strings.EqualFold(defaultProject, domainrunner.AllProjects) || strings.EqualFold(defaultProject, "all") || strings.EqualFold(defaultProject, "todos") {
		return
	}
	for index, project := range m.projects {
		if strings.EqualFold(project.Name, defaultProject) || strings.EqualFold(project.ID, defaultProject) {
			m.projectCursor = index + 1
			return
		}
	}
}

func (m contextModel) selectedProject() string {
	if m.projectCursor == 0 {
		return domainrunner.AllProjects
	}
	index := m.projectCursor - 1
	if index < 0 || index >= len(m.projects) {
		return domainrunner.AllProjects
	}
	return m.projects[index].Name
}

func (m contextModel) selectedProjectLabel() string {
	if m.projectCursor == 0 {
		return allProjectsLabel
	}
	index := m.projectCursor - 1
	if index < 0 || index >= len(m.projects) {
		return allProjectsLabel
	}
	return m.projects[index].Name
}

func (m contextModel) view() string {
	width := m.width
	if width == 0 {
		width = defaultWidth - 4
	}
	m.organization.Width = max(8, width-15)
	lines := []string{
		wordmarkStyle.Render("AZPIPE"),
		"",
		catalogTitleStyle.Render("Ligar ao Azure DevOps"),
	}
	if len(m.projects) == 0 {
		lines = append(lines,
			"Introduza a organização. A aplicação usa a sessão Azure DevOps configurada neste computador.",
			m.organization.View(),
		)
		if m.loading {
			lines = append(lines, catalogDetailStyle.Render("A validar credenciais e a carregar projectos..."))
		}
	} else {
		lines = append(lines,
			"Sessão autenticada. Escolha um projecto para abrir o catálogo de pipelines.",
			catalogDetailStyle.Render(truncateWidth("Organização: "+m.organization.Value(), defaultWidth)),
			catalogTitleStyle.Render(fmt.Sprintf("Projectos disponíveis · %d", len(m.projects))),
		)
		selected := "Seleccionado: " + m.selectedProjectLabel()
		selectedLines := strings.Count(ansi.Wrap(selected, width, ""), "\n") + 1
		extra := selectedLines - 1
		if m.err != "" {
			extra += 5
		}
		if m.loading {
			extra++
		}
		capacity := allProjectsCapacity
		if m.height > 0 {
			capacity = max(1, min(capacity, m.height-14-extra))
		}
		start := max(0, m.projectCursor-capacity+1)
		end := min(len(m.projects)+1, start+capacity)
		for index := start; index < end; index++ {
			label := allProjectsLabel
			if index > 0 {
				label = m.projects[index-1].Name
			}
			line := "  " + label
			if index == m.projectCursor {
				line = "> " + label
				line = catalogActiveStyle.Render(line)
			} else {
				line = catalogDetailStyle.Render(line)
			}
			lines = append(lines, truncateWidth(line, width))
		}
		lines = append(lines, catalogDetailStyle.Render(selected))
		if m.loading {
			lines = append(lines, catalogDetailStyle.Render("A carregar pipelines do âmbito seleccionado..."))
		}
	}
	if m.err != "" {
		lines = append(lines, "", catalogWarningStyle.Render(textPage(m.err, width, m.errorScroll, 3)), "PgUp/PgDn: percorrer erro completo")
	}
	if len(m.projects) > 0 {
		lines = append(lines, "", shortcutBar(width, "enter abrir catálogo"), shortcutBar(width, "↑/↓ escolher projecto", "esc mudar organização"))
	} else {
		lines = append(lines, "", shortcutBar(width, "enter ligar", "esc sair"))
	}
	return ansi.Wrap(strings.Join(lines, "\n"), width, "")
}

func loadProjects(factory ClientFactory, organization string, token operationToken) tea.Cmd {
	return func() tea.Msg {
		if factory == nil {
			return projectsLoadedMsg{token: token, organization: organization, err: fmt.Errorf("não foi possível criar o cliente: factory indisponível")}
		}
		client, err := factory(organization)
		if err != nil {
			return projectsLoadedMsg{token: token, organization: organization, err: fmt.Errorf("não foi possível autenticar na organização: %w", err)}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		projects, err := client.ListProjects(ctx)
		if err != nil {
			return projectsLoadedMsg{token: token, organization: organization, client: client, err: fmt.Errorf("não foi possível listar projectos: %w", err)}
		}
		return projectsLoadedMsg{token: token, organization: organization, client: client, projects: projects}
	}
}

func loadSelectedProject(client azdo.Client, organization, project string, projects []azdo.Project, token operationToken) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return contextLoadedMsg{token: token, organization: organization, project: project, err: fmt.Errorf("cliente Azure DevOps indisponível")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		var (
			pipelines []azdo.Pipeline
			err       error
		)
		if project == domainrunner.AllProjects {
			pipelines, err = listAllProjectPipelines(ctx, client, projects)
		} else {
			pipelines, err = client.ListPipelines(ctx, project)
			setPipelineProject(pipelines, project)
		}
		if err != nil {
			return contextLoadedMsg{token: token, organization: organization, client: client, project: project, err: err}
		}
		return contextLoadedMsg{token: token, organization: organization, client: client, project: project, pipelines: pipelines}
	}
}

func listAllProjectPipelines(ctx context.Context, client azdo.Client, projects []azdo.Project) ([]azdo.Pipeline, error) {
	if len(projects) == 0 {
		return []azdo.Pipeline{}, nil
	}
	results := make([][]azdo.Pipeline, len(projects))
	errorsByProject := make([]error, len(projects))
	jobs := make(chan int)
	workers := min(pipelineProjectParallel, len(projects))
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for index := range jobs {
				project := projects[index].Name
				pipelines, err := client.ListPipelines(ctx, project)
				if err != nil {
					errorsByProject[index] = fmt.Errorf("não foi possível listar pipelines do projecto %q: %w", project, err)
					continue
				}
				setPipelineProject(pipelines, project)
				results[index] = pipelines
			}
		}()
	}
	for index := range projects {
		jobs <- index
	}
	close(jobs)
	group.Wait()

	var all []azdo.Pipeline
	for index, pipelines := range results {
		if errorsByProject[index] != nil {
			return nil, errorsByProject[index]
		}
		all = append(all, pipelines...)
	}
	return all, nil
}

func setPipelineProject(pipelines []azdo.Pipeline, project string) {
	for index := range pipelines {
		pipelines[index].Project = project
	}
}

// loadContext remains available for callers that already have a project value.
// The bootstrap UI uses loadProjects followed by loadSelectedProject.
func loadContext(factory ClientFactory, organization, project string, token operationToken) tea.Cmd {
	return func() tea.Msg {
		if factory == nil {
			return contextLoadedMsg{token: token, organization: organization, project: project, err: fmt.Errorf("não foi possível criar o cliente: factory indisponível")}
		}
		client, err := factory(organization)
		if err != nil {
			return contextLoadedMsg{token: token, organization: organization, project: project, err: fmt.Errorf("não foi possível criar o cliente: %w", err)}
		}
		pipelines, err := client.ListPipelines(context.Background(), project)
		if err != nil {
			return contextLoadedMsg{token: token, organization: organization, client: client, project: project, err: fmt.Errorf("não foi possível listar pipelines: %w", err)}
		}
		setPipelineProject(pipelines, project)
		return contextLoadedMsg{token: token, organization: organization, client: client, project: project, pipelines: pipelines}
	}
}
