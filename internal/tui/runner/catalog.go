package runner

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ineslino/azpipe/internal/azdo"
	domainrunner "github.com/ineslino/azpipe/internal/runner"
	"github.com/mattn/go-runewidth"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
)

type catalogInput int

const (
	inputNone catalogInput = iota
	inputSearch
	inputBranch
	inputParameters
	inputParameterForm
)

// CatalogReviewMsg moves the selected pipelines to the review screen.
type CatalogReviewMsg struct {
	Selections []domainrunner.Selection
}

// CatalogModel lets an operator search and select pipelines before review.
type CatalogModel struct {
	pipelines      []azdo.Pipeline
	visible        []azdo.Pipeline
	selected       map[int]domainrunner.Mode
	search         textinput.Model
	branch         textinput.Model
	cursor         int
	width          int
	height         int
	input          catalogInput
	warning        string
	notice         string
	parameterInput textinput.Model
	parameters     map[int]map[string]string
	editor         parameterEditor
	branchBefore   string
	branches       map[int]string
}

// NewCatalogModel creates a catalog with all pipelines visible and main as branch.
func NewCatalogModel(pipelines []azdo.Pipeline) CatalogModel {
	search := textinput.New()
	search.Prompt = "Procurar: "
	search.Width = 40
	search.Placeholder = "nome, tipo, tag ou repositório"
	search.PromptStyle = keyStyle
	search.Cursor.Style = catalogActiveStyle
	search.CharLimit = 256

	branch := textinput.New()
	branch.Prompt = "Branch: "
	branch.SetValue("main")
	branch.Width = 40
	branch.CharLimit = 256

	model := CatalogModel{
		pipelines: slices.Clone(pipelines),
		selected:  make(map[int]domainrunner.Mode),
		search:    search,
		branch:    branch,
		width:     defaultWidth,
		height:    defaultHeight,
	}
	model.filter()
	model.parameterInput = textinput.New()
	model.parameterInput.Prompt = "Parâmetros JSON (sem segredos): "
	model.parameterInput.CharLimit = 4096
	model.parameterInput.Width = 60
	model.parameters = map[int]map[string]string{}
	model.branches = map[int]string{}
	return model
}

// Init performs no asynchronous work.
func (m CatalogModel) Init() tea.Cmd {
	return nil
}

// Selected returns the current selections in catalog order.
func (m CatalogModel) Selected() []domainrunner.Selection {
	selections := make([]domainrunner.Selection, 0, len(m.selected))
	for _, pipeline := range m.pipelines {
		mode, ok := m.selected[pipeline.ID]
		if !ok {
			continue
		}
		selections = append(selections, domainrunner.Selection{
			Pipeline: pipeline,
			Mode:     mode,
			Branch:   m.branchFor(pipeline.ID),
			Inputs:   m.parameters[pipeline.ID],
		})
	}
	return selections
}

func (m CatalogModel) branchFor(id int) string {
	if branch, ok := m.branches[id]; ok {
		return branch
	}
	return strings.TrimSpace(m.branch.Value())
}

// Update applies keyboard input and terminal dimensions to the catalog.
func (m CatalogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(1, typed.Width)
		m.height = max(1, typed.Height)
		return m, nil
	case tea.KeyMsg:
		if typed.Type == tea.KeyEsc {
			return m.escape()
		}
		if m.input != inputNone {
			return m.updateInput(msg)
		}
		return m.updateKey(typed)
	}
	return m, nil
}

func (m CatalogModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.notice = ""
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "/":
		m.input = inputSearch
		return m, m.search.Focus()
	case "b":
		m.branchBefore = m.branch.Value()
		m.input = inputBranch
		return m, m.branch.Focus()
	case "e":
		if pipeline, ok := m.active(); ok {
			m.editor = newParameterEditor(m.parameters[pipeline.ID])
			m.input = inputParameterForm
		}
		return m, nil
	case "J":
		if pipeline, ok := m.active(); ok {
			data, _ := json.Marshal(m.parameters[pipeline.ID])
			if string(data) == "null" {
				data = []byte("{}")
			}
			m.parameterInput.SetValue(string(data))
			m.input = inputParameters
			return m, m.parameterInput.Focus()
		}
	case "P", "R":
		if msg.String() == "P" {
			for _, pipeline := range m.pipelines {
				if _, selected := m.selected[pipeline.ID]; selected && pipeline.PlanContract == nil {
					m.warning = "PLAN global bloqueado: seleção contém pipeline sem contrato"
					return m, nil
				}
			}
		}
		for id := range m.selected {
			if msg.String() == "P" {
				m.selected[id] = domainrunner.ModePlan
			} else {
				m.selected[id] = domainrunner.ModeRun
			}
		}
	case "up", "k":
		m.cursor--
		m.clampCursor()
	case "down", "j":
		m.cursor++
		m.clampCursor()
	case " ":
		if pipeline, ok := m.active(); ok {
			if _, selected := m.selected[pipeline.ID]; selected {
				delete(m.selected, pipeline.ID)
			} else {
				m.selected[pipeline.ID] = domainrunner.ModeRun
			}
			m.warning = ""
		}
	case "m", "p":
		if pipeline, ok := m.active(); ok {
			if _, selected := m.selected[pipeline.ID]; !selected {
				m.warning = "Seleccione primeiro com espaço; depois altere o modo com m."
				return m, nil
			}
			if pipeline.PlanContract == nil {
				m.warning = "PLAN indisponível: contrato validado em falta"
				return m, nil
			}
			if m.selected[pipeline.ID] == domainrunner.ModePlan {
				m.selected[pipeline.ID] = domainrunner.ModeRun
			} else {
				m.selected[pipeline.ID] = domainrunner.ModePlan
			}
			m.warning = ""
		}
	case "enter":
		selections := m.Selected()
		if len(selections) == 0 {
			m.warning = "Selecione pelo menos uma pipeline antes de avançar."
			return m, nil
		}
		return m, func() tea.Msg { return CatalogReviewMsg{Selections: selections} }
	}
	return m, nil
}

func (m CatalogModel) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.input == inputParameterForm {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+s" {
			values, err := m.editor.values()
			if err != nil {
				m.editor.warning = err.Error()
				return m, nil
			}
			if pipeline, ok := m.active(); ok {
				m.parameters[pipeline.ID] = values
			}
			m.input = inputNone
			return m, nil
		}
		cmd := m.editor.update(msg)
		return m, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		if m.input == inputBranch {
			m.branches = map[int]string{}
		}
		if m.input == inputParameters {
			var params map[string]string
			if err := json.Unmarshal([]byte(m.parameterInput.Value()), &params); err != nil {
				m.warning = "Use um objecto JSON com valores string"
				return m, nil
			}
			if pipeline, ok := m.active(); ok {
				m.parameters[pipeline.ID] = params
			}
			m.parameterInput.Blur()
			m.warning = ""
		}
		m.search.Blur()
		m.branch.Blur()
		m.input = inputNone
		return m, nil
	}
	var cmd tea.Cmd
	switch m.input {
	case inputSearch:
		m.search, cmd = m.search.Update(msg)
		m.filter()
	case inputBranch:
		m.branch, cmd = m.branch.Update(msg)
	case inputParameters:
		m.parameterInput, cmd = m.parameterInput.Update(msg)
	}
	return m, cmd
}

func (m CatalogModel) escape() (tea.Model, tea.Cmd) {
	if m.input == inputBranch {
		m.branch.SetValue(m.branchBefore)
	}
	if m.input == inputSearch && m.search.Value() != "" {
		m.search.SetValue("")
		m.search.Blur()
		m.input = inputNone
		m.filter()
		return m, nil
	}
	if m.input != inputNone {
		m.search.Blur()
		m.branch.Blur()
		m.parameterInput.Blur()
		m.input = inputNone
		return m, nil
	}
	return m, tea.Quit
}

func (m *CatalogModel) filter() {
	query := strings.ToLower(strings.TrimSpace(m.search.Value()))
	m.visible = m.visible[:0]
	for _, pipeline := range m.pipelines {
		if query == "" || strings.Contains(searchablePipeline(pipeline), query) {
			m.visible = append(m.visible, pipeline)
		}
	}
	m.clampCursor()
}

func searchablePipeline(pipeline azdo.Pipeline) string {
	return strings.ToLower(strings.Join([]string{
		pipeline.Name,
		strconv.Itoa(pipeline.ID),
		pipeline.Folder,
		pipeline.Type(),
		pipeline.RepoName,
		strings.Join(pipeline.Tags, " "),
	}, "\n"))
}

func (m *CatalogModel) clampCursor() {
	if len(m.visible) == 0 {
		m.cursor = 0
		return
	}
	m.cursor = min(max(0, m.cursor), len(m.visible)-1)
}

func (m CatalogModel) active() (azdo.Pipeline, bool) {
	if len(m.visible) == 0 || m.cursor < 0 || m.cursor >= len(m.visible) {
		return azdo.Pipeline{}, false
	}
	return m.visible[m.cursor], true
}

// View renders compact rows and reserves a detail line for the active pipeline.
func (m CatalogModel) View() string {
	if m.input == inputParameterForm {
		pipeline, _ := m.active()
		return section("CONFIGURAR PARÂMETROS", m.editor.view(max(1, m.width-4), max(1, m.height-2), pipeline.Name), m.width)
	}
	plans := 0
	for _, mode := range m.selected {
		if mode == domainrunner.ModePlan {
			plans++
		}
	}
	branchLabel := m.branch.Value()
	if len(m.branches) > 0 {
		branchLabel = "por pipeline (perfil)"
	}
	lines := []string{
		catalogTitleStyle.Render(fmt.Sprintf("Pipelines · %d seleccionadas · %d visíveis", len(m.selected), len(m.visible))),
		runStyle.Render(fmt.Sprintf("%d RUN", len(m.selected)-plans)) + "  " + planStyle.Render(fmt.Sprintf("%d PLAN", plans)) + "  " + catalogDetailStyle.Render(truncateWidth("Branch: "+branchLabel, max(1, m.width-20))),
		m.search.View(),
	}

	start, end := m.displayRange()
	inner := max(1, m.width-4)
	widths := []int{4, 4, 5, 10, max(1, inner-35)}
	headers := []string{"SEL", "MODO", "ID", "TIPO", "PIPELINE"}
	if m.width < 70 {
		widths = []int{4, 4, 5, max(1, inner-22)}
		headers = []string{"SEL", "MODO", "ID", "PIPELINE"}
	}
	rows := []string{catalogHeaderStyle.Width(inner).Render(tableCells(widths, headers...))}
	if len(m.visible) == 0 {
		rows = append(rows, catalogDetailStyle.Render("Nenhuma pipeline encontrada."))
	}
	for index := start; index < end; index++ {
		pipeline := m.visible[index]
		marker := " [ ]"
		mode := "-"
		if selectedMode, ok := m.selected[pipeline.ID]; ok {
			marker = " [x]"
			mode = string(selectedMode)
		}
		if index == m.cursor {
			marker = ">" + marker[1:]
		}
		values := []string{marker, mode, strconv.Itoa(pipeline.ID), pipeline.Type(), pipeline.Name}
		if m.width < 70 {
			values = []string{marker, mode, strconv.Itoa(pipeline.ID), pipeline.Name}
		}
		row := tableCells(widths, values...)
		if index == m.cursor {
			row = catalogActiveStyle.Width(inner).Render(row)
		} else if mode, selected := m.selected[pipeline.ID]; selected {
			row = modeStyle(string(mode)).Render(row)
		} else if index%2 == 0 {
			row = stripeStyle.Width(inner).Render(row)
		}
		rows = append(rows, row)
	}
	for len(rows) < m.catalogCapacity()+1 {
		rows = append(rows, "")
	}
	lines = append(lines, section(fmt.Sprintf("PIPELINES %d–%d / %d", min(start+1, len(m.visible)), end, len(m.visible)), strings.Join(rows, "\n"), m.width))
	detail := "Nenhuma pipeline activa.\nAltere o filtro para ver resultados."
	if pipeline, ok := m.active(); ok {
		capability := "PLAN indisponível: sem contrato validado"
		if pipeline.PlanContract != nil {
			capability = "PLAN disponível por contrato"
		}
		detail = catalogDetailStyle.Render(strings.TrimSpace(m.pipelineDetail(pipeline))) + "\n" + planStyle.Render(capability) + catalogDetailStyle.Render(fmt.Sprintf(" · %d parâmetros", len(m.parameters[pipeline.ID])))
	}
	lines = append(lines, section("DETALHE DA PIPELINE ACTIVA", detail, m.width))
	if m.input == inputBranch {
		lines = append(lines, m.branch.View())
	}
	if m.input == inputParameters {
		lines = append(lines, m.parameterInput.View())
	}
	if m.warning != "" {
		lines = append(lines, catalogWarningStyle.Render(m.warning))
	}
	if m.notice != "" {
		lines = append(lines, catalogDetailStyle.Render(truncateWidth(m.notice, m.width)))
	}
	lines = append(lines, section("ACÇÕES", m.helpView(), m.width))
	return strings.Join(lines, "\n")
}

func (m CatalogModel) displayRange() (int, int) {
	if len(m.visible) == 0 {
		return 0, 0
	}
	available := m.catalogCapacity()
	start := max(0, m.cursor-available+1)
	end := min(len(m.visible), start+available)
	return start, end
}

func (m CatalogModel) catalogCapacity() int {
	available := m.height - 15 - lipgloss.Height(m.helpView())
	if m.warning != "" {
		available--
	}
	if m.notice != "" {
		available--
	}
	if m.input == inputBranch || m.input == inputParameters {
		available--
	}
	return max(1, available)
}

func (m CatalogModel) helpView() string {
	if m.input != inputNone {
		return shortcutBar(max(1, m.width-4), "enter guardar e voltar à lista", "esc cancelar edição")
	}
	return shortcutBar(max(1, m.width-4), "↑/↓ navegar", "espaço seleccionar", "enter rever", "/ filtrar", "m modo", "e parâmetros", "b branch", "s guardar perfil", "l perfis", "h lotes", "P/R modo global", "q sair")
}

func (m CatalogModel) pipelineRow(pipeline azdo.Pipeline, active bool) string {
	marker := " "
	check := "[ ]"
	if active {
		marker = ">"
	}
	mode := "-"
	if selectedMode, selected := m.selected[pipeline.ID]; selected {
		check = "[x]"
		mode = string(selectedMode)
	}
	nameWidth := max(1, m.width-32)
	if m.width < 60 {
		return truncateWidth(fmt.Sprintf("%s%s %-4s %d %s", marker, check, mode, pipeline.ID, pipeline.Name), m.width)
	}
	return fmt.Sprintf("%s%s %-4s %5d %-12s %s", marker, check, mode, pipeline.ID, truncateWidth(pipeline.Type(), 12), truncateWidth(pipeline.Name, nameWidth))
}

func (m CatalogModel) pipelineDetail(pipeline azdo.Pipeline) string {
	detail := fmt.Sprintf("    repo: %s | folder: %s | tags: %s", pipeline.RepoName, pipeline.Folder, strings.Join(pipeline.Tags, ", "))
	if pipeline.MetadataWarning != "" {
		detail = "    ⚠ " + pipeline.MetadataWarning + " | " + detail
	}
	return truncateWidth(detail, m.width)
}

func truncateWidth(value string, width int) string {
	if width <= 0 || runewidth.StringWidth(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	var builder strings.Builder
	used := 0
	for _, r := range value {
		runeWidth := runewidth.RuneWidth(r)
		if used+runeWidth > width-1 {
			break
		}
		builder.WriteRune(r)
		used += runeWidth
	}
	return builder.String() + "…"
}

func horizontalWindow(value string, offset, width int) string {
	runes := []rune(value)
	if offset >= len(runes) {
		return ""
	}
	return truncateWidth(string(runes[offset:]), width)
}
