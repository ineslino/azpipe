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
	"github.com/charmbracelet/lipgloss"
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
	branchesOnly   bool
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
	organization.TextStyle = catalogTextStyle
	organization.Placeholder = "nome ou https://dev.azure.com/organização"
	organization.PlaceholderStyle = catalogDetailStyle
	organization.SetValue(defaults.Organization)
	organization.Focus()
	project := textinput.New()
	project.SetValue(defaults.Project)
	project.Blur()

	return contextModel{
		width:          defaultWidth,
		height:         defaultHeight,
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
			return m, nil
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
			case "pgup":
				m.projectCursor = max(0, m.projectCursor-m.projectCapacity())
				return m, nil
			case "pgdown":
				m.projectCursor = min(len(m.projects), m.projectCursor+m.projectCapacity())
				return m, nil
			case "home":
				m.projectCursor = 0
				return m, nil
			case "end":
				m.projectCursor = len(m.projects)
				return m, nil
			case "enter":
				m.err = ""
				m.errorScroll = 0
				m.loading = true
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
			m.errorScroll = 0
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
	inner := max(1, m.width-4)
	if len(m.projects) > 0 {
		capacity := m.projectCapacity()
		start := max(0, m.projectCursor-capacity+1)
		end := min(len(m.projects)+1, start+capacity)
		widths := []int{3, max(1, inner-21), 12}
		var rows []string
		for index := start; index < end; index++ {
			label, scope := allProjectsLabel, "ORGANIZAÇÃO"
			if index > 0 {
				label, scope = m.projects[index-1].Name, "PROJECTO"
			}
			marker := ""
			if index == m.projectCursor {
				marker = ">"
			}
			rows = append(rows, renderTableRow(widths, []string{marker, label, scope}, index, index == m.projectCursor, nil))
		}
		return m.projectView(rows, start, end)
	}
	m.organization.Width = max(8, inner-ansi.StringWidth(m.organization.Prompt)-1)
	body := catalogDetailStyle.Render(ansi.Wrap("Usa a sessão Azure DevOps configurada neste computador.", inner, "")) + "\n" + m.organization.View()
	if m.loading {
		body += "\n" + catalogDetailStyle.Render("A validar credenciais e a carregar projectos...")
	}
	return m.frame(section("1 · LIGAR AO AZURE DEVOPS", body, m.width))
}

func (m contextModel) projectView(rows []string, start, end int) string {
	inner := max(1, m.width-4)
	contextWidths := []int{12, max(1, inner-15)}
	context := tableCells(contextWidths, catalogDetailStyle.Render("Organização"), m.organization.Value()) + "\n" +
		tableCells(contextWidths, catalogDetailStyle.Render("Sessão"), successStyle.Render("Autenticada")+fmt.Sprintf(" · %d projectos", len(m.projects)))
	widths := []int{3, max(1, inner-21), 12}
	selected := metadata("Seleccionado", m.selectedProjectLabel())
	if m.projectCursor == 0 {
		hint := "Catálogo de toda a organização."
		if m.branchesOnly {
			hint = "Escolhe um projecto para gerir branches."
		}
		selected += "\n" + catalogDetailStyle.Render(hint)
	}
	table := []string{catalogHeaderStyle.Width(inner).Render(tableCells(widths, "SEL", "PROJECTO", "ÂMBITO"))}
	table = append(table, rows...)
	table = append(table, borderStyle.Render(strings.Repeat("─", inner)), ansi.Wrap(selected, inner, ""))
	if m.loading {
		table = append(table, catalogDetailStyle.Render("A abrir a selecção..."))
	}
	return m.frame(
		section("CONTEXTO", context, m.width),
		section(fmt.Sprintf("2 · ESCOLHER PROJECTO · %d–%d / %d", start+1, end, len(m.projects)+1), strings.Join(table, "\n"), m.width),
	)
}

func (m contextModel) projectCapacity() int {
	// Measure the same sections we render, including wrapped names and diagnostics.
	fixed := lipgloss.Height(m.projectView(nil, 0, 0))
	return max(1, min(allProjectsCapacity, m.height-fixed))
}

func (m contextModel) frame(panels ...string) string {
	inner := max(1, m.width-4)
	brand := wordmarkStyle.Render("AZPIPE") + "  " + catalogDetailStyle.Render("AZURE DEVOPS / TUI")
	if len(m.projects) == 0 && m.width >= 60 && m.height >= 24 {
		brand = welcomeBrand()
	}
	if m.err != "" {
		count := 3
		if len(m.projects) > 0 {
			count = 2
			if m.height < defaultHeight {
				count = 1
			}
		}
		body := catalogWarningStyle.Render(textPage(m.err, inner, m.errorScroll, count)) + "\n" + shortcutBar(inner, "PgUp/PgDn percorrer erro")
		panels = append(panels, section("ERRO", body, m.width))
	}
	help := shortcutBar(inner, "enter ligar", ":q sair")
	if len(m.projects) > 0 {
		primary := "enter abrir catálogo"
		if m.branchesOnly {
			primary = "enter abrir repositórios"
		}
		help = shortcutBar(inner, primary) + "\n" +
			shortcutBar(inner, "↑/↓ escolher", "PgUp/PgDn página", "Home/End extremos", "esc mudar organização", ":q sair")
	}
	if m.loading {
		help = catalogDetailStyle.Render("Aguarda o carregamento.") + "\n" + shortcutBar(inner, ":q sair")
	}
	panels = append(panels, section("ACÇÕES", help, m.width))
	prefix := brand + "\n\n"
	view := prefix + strings.Join(panels, "\n\n")
	if lipgloss.Height(view) > m.height {
		view = prefix + strings.Join(panels, "\n")
	}
	return view
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
