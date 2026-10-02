package runner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
)

// ClientFactory creates an authenticated client after the operator submits an organization.
type ClientFactory func(organization string) (azdo.Client, error)

// ContextDefaults pre-fills non-secret context values from flags or configuration.
type ContextDefaults struct {
	Organization string
	Project      string
}

type contextSubmitMsg struct {
	generation   uint64
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
	generation       uint64
	width, height    int
	branchesOnly     bool
	errorScroll      int
	organization     textinput.Model
	project          textinput.Model // retained for compatibility with the non-interactive bootstrap contract
	projects         []azdo.Project
	projectCursor    int
	projectDefault   string
	projectSearch    textinput.Model
	projectSearching bool
	focus            int
	loading          bool
	err              string
	recovery         string
	notice           string
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
	search := textinput.New()
	search.Prompt = "Procurar: "
	search.Placeholder = "projecto ou ID · / editar"
	search.CharLimit = 256
	search.TextStyle, search.PlaceholderStyle = catalogTextStyle, catalogDetailStyle
	search.PromptStyle = keyStyle

	return contextModel{
		width:          defaultWidth,
		height:         defaultHeight,
		organization:   organization,
		project:        project,
		projectDefault: strings.TrimSpace(defaults.Project),
		projectSearch:  search,
	}
}

func (m contextModel) update(msg tea.Msg) (contextModel, tea.Cmd) {
	key, isKey := msg.(tea.KeyMsg)
	if isKey {
		if m.projectSearching {
			if key.Type == tea.KeyEsc || key.Type == tea.KeyEnter {
				m.projectSearching = false
				m.projectSearch.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.projectSearch, cmd = m.projectSearch.Update(key)
			m.selectVisibleProject()
			return m, cmd
		}
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
			if key.Type == tea.KeyRunes && !key.Alt && len(key.Runes) > 0 && key.Runes[0] == '/' {
				m.projectSearching = true
				focus := m.projectSearch.Focus()
				if len(key.Runes) > 1 {
					var input tea.Cmd
					m.projectSearch, input = m.projectSearch.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: key.Runes[1:], Paste: key.Paste})
					m.selectVisibleProject()
					return m, tea.Batch(focus, input)
				}
				return m, focus
			}
			indexes := m.visibleProjectIndexes()
			position := m.visibleProjectPosition(indexes)
			switch key.String() {
			case "c":
				m.projectSearch.SetValue("")
				m.selectVisibleProject()
				return m, nil
			case "up", "k", "shift+tab":
				position--
			case "down", "j", "tab":
				position++
			case "pgup":
				position -= m.projectCapacity()
			case "pgdown":
				position += m.projectCapacity()
			case "home":
				position = 0
			case "end":
				position = len(indexes) - 1
			case "enter":
				if len(indexes) == 0 {
					return m, nil
				}
				m.err = ""
				m.recovery, m.notice = "", ""
				m.errorScroll = 0
				m.loading = true
				organization := strings.TrimSpace(m.organization.Value())
				return m, func() tea.Msg {
					return contextSubmitMsg{generation: m.generation, organization: organization, project: m.selectedProject()}
				}
			default:
				return m, nil
			}
			if len(indexes) > 0 {
				m.projectCursor = indexes[min(max(0, position), len(indexes)-1)]
			}
			return m, nil
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
				m.recovery = "Introduz o nome ou o URL da organização e prime Enter."
				return m, nil
			}
			m.err = ""
			m.recovery, m.notice = "", ""
			m.errorScroll = 0
			m.loading = true
			return m, func() tea.Msg {
				return contextSubmitMsg{generation: m.generation, organization: organization}
			}
		}
	}

	var cmd tea.Cmd
	if m.projectSearching {
		m.projectSearch, cmd = m.projectSearch.Update(msg)
		return m, cmd
	}
	if m.focus == contextOrganizationFocus {
		m.organization, cmd = m.organization.Update(msg)
	}
	return m, cmd
}

func (m *contextModel) setFocus(focus int) {
	m.focus = focus
	if focus == contextOrganizationFocus {
		m.projectSearch.Blur()
		m.projectSearching = false
		m.project.Blur()
		m.organization.Focus()
		return
	}
	m.organization.Blur()
	m.project.Blur()
}

func (m *contextModel) setProjects(projects []azdo.Project) {
	m.projectSearch.SetValue("")
	m.projectSearch.Blur()
	m.projectSearching = false
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

func (m contextModel) visibleProjectIndexes() []int {
	query := strings.ToLower(strings.TrimSpace(m.projectSearch.Value()))
	var indexes []int
	if query == "" || strings.Contains(strings.ToLower(allProjectsLabel), query) {
		indexes = append(indexes, 0)
	}
	for index, project := range m.projects {
		if query == "" || strings.Contains(strings.ToLower(project.Name), query) || strings.Contains(strings.ToLower(project.ID), query) {
			indexes = append(indexes, index+1)
		}
	}
	return indexes
}

func (m contextModel) visibleProjectPosition(indexes []int) int {
	for position, index := range indexes {
		if index == m.projectCursor {
			return position
		}
	}
	return 0
}

func (m *contextModel) selectVisibleProject() {
	indexes := m.visibleProjectIndexes()
	if len(indexes) == 0 {
		m.projectCursor = -1
		return
	}
	m.projectCursor = indexes[m.visibleProjectPosition(indexes)]
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
	if m.projectCursor < 0 {
		return "Nenhum projecto"
	}
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
		indexes := m.visibleProjectIndexes()
		capacity := m.projectCapacity()
		start := max(0, m.visibleProjectPosition(indexes)-capacity+1)
		end := min(len(indexes), start+capacity)
		widths := []int{3, max(1, inner-21), 12}
		var rows []string
		for _, index := range indexes[start:end] {
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
	if m.notice != "" {
		body += "\n" + catalogDetailStyle.Render(ansi.Wrap(m.notice, inner, ""))
	}
	return m.frame(section("1 · LIGAR AO AZURE DEVOPS", body, m.width))
}

func (m contextModel) projectView(rows []string, start, end int) string {
	inner := max(1, m.width-4)
	contextWidths := []int{12, max(1, inner-15)}
	context := tableCells(contextWidths, catalogDetailStyle.Render("Organização"), m.organization.Value()) + "\n" +
		tableCells(contextWidths, catalogDetailStyle.Render("Sessão"), successStyle.Render("Autenticada")+fmt.Sprintf(" · %d projectos", len(m.projects)))
	if m.err != "" && m.height < 28 {
		context = catalogTextStyle.Render(fmt.Sprintf("%s · %d projectos · Autenticada", m.organization.Value(), len(m.projects)))
	}
	widths := []int{3, max(1, inner-21), 12}
	selected := metadata("Seleccionado", m.selectedProjectLabel())
	if m.projectCursor == 0 {
		hint := "Catálogo de toda a organização."
		if m.branchesOnly {
			hint = "Escolhe um projecto para gerir branches."
		}
		selected += "\n" + catalogDetailStyle.Render(hint)
	}
	var table []string
	if m.err == "" || m.projectSearch.Value() != "" {
		m.projectSearch.Width = max(8, inner-10)
		table = append(table, m.projectSearch.View())
	}
	table = append(table, catalogHeaderStyle.Width(inner).Render(tableCells(widths, "SEL", "PROJECTO", "ÂMBITO")))
	table = append(table, rows...)
	if len(m.visibleProjectIndexes()) == 0 {
		table = append(table, catalogDetailStyle.Render("Nenhum projecto corresponde à pesquisa. c limpa o filtro."))
	}
	table = append(table, borderStyle.Render(strings.Repeat("─", inner)), ansi.Wrap(selected, inner, ""))
	if m.loading {
		operation := "A carregar pipelines do projecto seleccionado..."
		if m.projectCursor == 0 {
			operation = fmt.Sprintf("A carregar pipelines de %d projectos...", len(m.projects))
		}
		table = append(table, catalogDetailStyle.Render(ansi.Wrap(operation, inner, "")))
	}
	if m.notice != "" {
		table = append(table, catalogDetailStyle.Render(ansi.Wrap(m.notice, inner, "")))
	}
	return m.frame(
		section("CONTEXTO", context, m.width),
		section(fmt.Sprintf("2 · ESCOLHER PROJECTO · %d–%d / %d", min(start+1, len(m.visibleProjectIndexes())), end, len(m.visibleProjectIndexes())), strings.Join(table, "\n"), m.width),
	)
}

func (m contextModel) projectCapacity() int {
	// Measure the same sections we render, including wrapped names and diagnostics.
	fixed := lipgloss.Height(m.projectView(nil, 0, 0))
	return max(1, min(allProjectsCapacity, m.height-fixed))
}

func (m contextModel) frame(panels ...string) string {
	inner := max(1, m.width-4)
	compactBrand := wordmarkStyle.Render("AZPIPE") + "  " + catalogDetailStyle.Render("AZURE DEVOPS / TUI")
	brand := compactBrand
	if len(m.projects) == 0 && m.width >= 60 && m.height >= 24 {
		brand = welcomeBrand()
	}
	if m.err != "" {
		count := 3
		if len(m.projects) > 0 {
			count = 1
		}
		recovery := m.recovery
		if recovery == "" {
			recovery = "Confirma a organização e o acesso com a sessão configurada. Depois tenta novamente."
		}
		body := catalogWarningStyle.Render(textPage(m.err, inner, m.errorScroll, count)) + "\n" +
			catalogTitleStyle.Render("COMO RECUPERAR") + "\n" + catalogDetailStyle.Render(ansi.Wrap(recovery, inner, "")) + "\n" + shortcutBar(inner, "PgUp/PgDn percorrer erro")
		panels = append(panels, section("ERRO", body, m.width))
	}
	help := shortcutBar(inner, "enter ligar", ":q sair")
	if len(m.projects) > 0 {
		primary := "enter abrir catálogo"
		if m.branchesOnly {
			primary = "enter abrir repositórios"
		}
		help = shortcutBar(inner, primary) + "\n" + shortcutBar(inner, "↑/↓ escolher", "/ procurar", "c limpar", "esc mudar organização", ":q sair")
		if m.height >= 28 && m.err == "" {
			help += "\n" + shortcutBar(inner, "PgUp/PgDn página", "Home/End extremos")
		} else if m.err == "" {
			help = shortcutBar(inner, primary) + "\n" + shortcutBar(inner, "↑/↓ escolher", "PgUp/PgDn página", "/ procurar", "esc mudar organização", ":q sair")
		}
		if m.projectSearching {
			help = shortcutBar(inner, "enter terminar pesquisa", "esc voltar à lista", ":q sair")
		} else if m.err != "" {
			help = shortcutBar(inner, primary) + "\n" + shortcutBar(inner, "esc mudar organização", ":q sair")
		}
	}
	if m.loading {
		help = shortcutBar(inner, "esc cancelar leitura", ":q sair")
	}
	panels = append(panels, section("ACÇÕES", help, m.width))
	prefix := brand + "\n\n"
	view := prefix + strings.Join(panels, "\n\n")
	if lipgloss.Height(view) > m.height {
		view = prefix + strings.Join(panels, "\n")
	}
	if lipgloss.Height(view) > m.height {
		view = brand + "\n" + strings.Join(panels, "\n")
	}
	if lipgloss.Height(view) > m.height {
		view = compactBrand + "\n" + strings.Join(panels, "\n")
	}
	return view
}

func (m *contextModel) setError(err error) {
	m.err = err.Error()
	m.recovery = "Confirma a organização e a sessão configurada. Renova as credenciais pelo login aprovado, se necessário."
	if errors.Is(err, context.DeadlineExceeded) {
		m.recovery = "Tempo de espera excedido. Verifica a ligação à rede e tenta novamente."
		return
	}
	var pointer *azuredevops.WrappedError
	var value azuredevops.WrappedError
	status := 0
	if errors.As(err, &pointer) && pointer != nil && pointer.StatusCode != nil {
		status = *pointer.StatusCode
	} else if errors.As(err, &value) && value.StatusCode != nil {
		status = *value.StatusCode
	}
	switch status {
	case http.StatusUnauthorized:
		m.recovery = "Renova as credenciais pelo login aprovado e confirma a sessão configurada. Depois tenta novamente."
	case http.StatusForbidden:
		m.recovery = "Verifica as permissões da sessão para esta organização e projecto. Depois tenta novamente."
	case http.StatusNotFound:
		m.recovery = "Confirma a organização e o projecto. Se existirem, verifica o acesso da sessão."
	case http.StatusTooManyRequests:
		m.recovery = "Aguarda antes de repetir. O Azure DevOps limitou a frequência dos pedidos."
	}
}

func loadProjects(parent context.Context, factory ClientFactory, organization string, token operationToken) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		defer cancel()
		if err := ctx.Err(); err != nil {
			return projectsLoadedMsg{token: token, organization: organization, err: err}
		}
		if factory == nil {
			return projectsLoadedMsg{token: token, organization: organization, err: fmt.Errorf("não foi possível criar o cliente: factory indisponível")}
		}
		client, err := factory(organization)
		if err != nil {
			return projectsLoadedMsg{token: token, organization: organization, err: fmt.Errorf("não foi possível autenticar na organização: %w", err)}
		}
		if err := ctx.Err(); err != nil {
			return projectsLoadedMsg{token: token, organization: organization, err: err}
		}
		projects, err := client.ListProjects(ctx)
		if err != nil {
			return projectsLoadedMsg{token: token, organization: organization, client: client, err: fmt.Errorf("não foi possível listar projectos: %w", err)}
		}
		return projectsLoadedMsg{token: token, organization: organization, client: client, projects: projects}
	}
}

func loadSelectedProject(parent context.Context, client azdo.Client, organization, project string, projects []azdo.Project, token operationToken) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return contextLoadedMsg{token: token, organization: organization, project: project, err: fmt.Errorf("cliente Azure DevOps indisponível")}
		}
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		if err := ctx.Err(); err != nil {
			return contextLoadedMsg{token: token, organization: organization, project: project, err: err}
		}
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
				if err := ctx.Err(); err != nil {
					errorsByProject[index] = err
					continue
				}
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}

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
func loadContext(parent context.Context, factory ClientFactory, organization, project string, token operationToken) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
		defer cancel()
		if err := ctx.Err(); err != nil {
			return contextLoadedMsg{token: token, organization: organization, project: project, err: err}
		}
		if factory == nil {
			return contextLoadedMsg{token: token, organization: organization, project: project, err: fmt.Errorf("não foi possível criar o cliente: factory indisponível")}
		}
		client, err := factory(organization)
		if err != nil {
			return contextLoadedMsg{token: token, organization: organization, project: project, err: fmt.Errorf("não foi possível criar o cliente: %w", err)}
		}
		if err := ctx.Err(); err != nil {
			return contextLoadedMsg{token: token, organization: organization, project: project, err: err}
		}
		pipelines, err := client.ListPipelines(ctx, project)
		if err != nil {
			return contextLoadedMsg{token: token, organization: organization, client: client, project: project, err: fmt.Errorf("não foi possível listar pipelines: %w", err)}
		}
		setPipelineProject(pipelines, project)
		return contextLoadedMsg{token: token, organization: organization, client: client, project: project, pipelines: pipelines}
	}
}
