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
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
	domainrunner "github.com/ineslino/azpipe/internal/runner"
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
	showDetails    bool
	detailScroll   int
	pipelines      []azdo.Pipeline
	visible        []azdo.Pipeline
	selected       map[string]domainrunner.Mode
	search         textinput.Model
	branch         textinput.Model
	cursor         int
	width          int
	height         int
	input          catalogInput
	warning        string
	notice         string
	parameterInput textinput.Model
	parameters     map[string]map[string]string
	editor         parameterEditor
	branchBefore   string
	branches       map[string]string
}

// NewCatalogModel creates a catalog with all pipelines visible and main as branch.
func NewCatalogModel(pipelines []azdo.Pipeline) CatalogModel {
	search := textinput.New()
	search.Prompt = "Procurar: "
	search.Width = 40
	search.Placeholder = "nome, tipo, tag ou repositório"
	search.PromptStyle = keyStyle
	search.TextStyle, search.PlaceholderStyle = catalogTextStyle, catalogDetailStyle
	search.Cursor.Style = catalogActiveStyle
	search.CharLimit = 256

	branch := textinput.New()
	branch.Prompt = "Branch global: "
	branch.SetValue("main")
	branch.Width = 40
	branch.CharLimit = 256

	model := CatalogModel{
		pipelines: slices.Clone(pipelines),
		selected:  make(map[string]domainrunner.Mode),
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
	model.parameters = map[string]map[string]string{}
	model.branches = map[string]string{}
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
		key := domainrunner.PipelineKey(pipeline)
		mode, ok := m.selected[key]
		if !ok {
			continue
		}
		selections = append(selections, domainrunner.Selection{
			Pipeline: pipeline,
			Mode:     mode,
			Branch:   m.branchFor(pipeline),
			Inputs:   m.parameters[key],
		})
	}
	return selections
}

func (m CatalogModel) branchFor(pipeline azdo.Pipeline) string {
	if branch, ok := m.branches[domainrunner.PipelineKey(pipeline)]; ok {
		return branch
	}
	return strings.TrimSpace(m.branch.Value())
}

// Update applies keyboard input and terminal dimensions to the catalog.
func (m CatalogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && m.showDetails {
		switch key.String() {
		case "esc", "d":
			m.showDetails = false
		case "down", "pgdown":
			m.detailScroll++
		case "up", "pgup":
			m.detailScroll = max(0, m.detailScroll-1)
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "d" && m.input == inputNone {
		m.showDetails = true
		m.detailScroll = 0
		return m, nil
	}
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(1, typed.Width)
		m.height = max(1, typed.Height)
		return m, nil
	case tea.KeyMsg:
		if typed.Type == tea.KeyEsc {
			if m.input == inputParameterForm && m.editor.optionsOpen {
				m.editor.update(msg)
				return m, nil
			}
			return m.escape()
		}
		if m.input != inputNone {
			return m.updateInput(msg)
		}
		if typed.Type == tea.KeyRunes && !typed.Alt && len(typed.Runes) > 1 && typed.Runes[0] == '/' {
			updated, focus := m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
			m = updated.(CatalogModel)
			typed.Runes = typed.Runes[1:]
			updated, input := m.updateInput(typed)
			return updated, tea.Batch(focus, input)
		}
		return m.updateKey(typed)
	}
	return m, nil
}

func (m CatalogModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.notice = ""
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "x":
		visible := make(map[string]bool, len(m.visible))
		for _, pipeline := range m.visible {
			visible[domainrunner.PipelineKey(pipeline)] = true
		}
		for key := range m.selected {
			if !visible[key] {
				delete(m.selected, key)
			}
		}
	case "A":
		count := len(m.selected)
		for _, p := range m.visible {
			if _, selected := m.selected[domainrunner.PipelineKey(p)]; !selected {
				count++
			}
		}
		if err := domainrunner.ValidateBatchSize(count); err != nil {
			m.warning = err.Error()
			return m, nil
		}
		for _, pipeline := range m.visible {
			key := domainrunner.PipelineKey(pipeline)
			if _, selected := m.selected[key]; !selected {
				m.selected[key] = domainrunner.ModeRun
			}
		}
		m.warning = ""
	case "/":
		m.input = inputSearch
		return m, m.search.Focus()
	case "b":
		m.branchBefore = m.branch.Value()
		m.input = inputBranch
		return m, m.branch.Focus()
	case "e":
		if pipeline, ok := m.active(); ok {
			m.editor = newParameterEditor(m.parameters[domainrunner.PipelineKey(pipeline)])
			m.input = inputParameterForm
		}
		return m, nil
	case "J":
		if pipeline, ok := m.active(); ok {
			data, _ := json.Marshal(m.parameters[domainrunner.PipelineKey(pipeline)])
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
				if _, selected := m.selected[domainrunner.PipelineKey(pipeline)]; selected && pipeline.PlanContract == nil {
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
			key := domainrunner.PipelineKey(pipeline)
			if _, selected := m.selected[key]; selected {
				delete(m.selected, key)
			} else {
				if len(m.selected) >= domainrunner.MaxBatchSize {
					m.warning = "O lote já tem 500 pipelines. Remove uma selecção antes de continuar."
					return m, nil
				}
				m.selected[key] = domainrunner.ModeRun
			}
			m.warning = ""
		}
	case "m", "p":
		if pipeline, ok := m.active(); ok {
			key := domainrunner.PipelineKey(pipeline)
			if _, selected := m.selected[key]; !selected {
				m.warning = "Seleccione primeiro com espaço; depois altere o modo com m."
				return m, nil
			}
			if pipeline.PlanContract == nil {
				m.warning = "PLAN indisponível: contrato validado em falta"
				return m, nil
			}
			if m.selected[key] == domainrunner.ModePlan {
				m.selected[key] = domainrunner.ModeRun
			} else {
				m.selected[key] = domainrunner.ModePlan
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
		if m.editor.optionsOpen {
			return m, m.editor.update(msg)
		}
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+s" {
			values, err := m.editor.values()
			if err != nil {
				m.editor.setWarning(err)
				return m, nil
			}
			if pipeline, ok := m.active(); ok {
				m.parameters[domainrunner.PipelineKey(pipeline)] = values
			}
			m.input = inputNone
			return m, nil
		}
		cmd := m.editor.update(msg)
		return m, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		if m.input == inputBranch {
			m.branches = map[string]string{}
		}
		if m.input == inputParameters {
			var params map[string]string
			if err := json.Unmarshal([]byte(m.parameterInput.Value()), &params); err != nil {
				m.warning = "Use um objecto JSON com valores string"
				return m, nil
			}
			if pipeline, ok := m.active(); ok {
				m.parameters[domainrunner.PipelineKey(pipeline)] = params
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
	if m.input != inputNone {
		m.search.Blur()
		m.branch.Blur()
		m.parameterInput.Blur()
		m.input = inputNone
		return m, nil
	}
	return m, nil
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
		pipeline.Project,
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

func (m CatalogModel) selectionView() string {
	inner := max(1, m.width-4)
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
	m.search.Width = max(8, inner-10)
	lines := []string{
		catalogTitleStyle.Render(truncateWidth(m.nextStep(), inner)),
		tableCells([]int{9, 9, max(1, inner-24)}, runStyle.Render(fmt.Sprintf("%d RUN", len(m.selected)-plans)), planStyle.Render(fmt.Sprintf("%d PLAN", plans)), metadata("Branch", branchLabel)),
		m.search.View(),
	}
	if m.input == inputBranch {
		m.branch.Width = max(8, inner-ansi.StringWidth(m.branch.Prompt)-1)
		lines = append(lines, m.branch.View())
	}
	if m.input == inputParameters {
		m.parameterInput.Width = max(8, inner-32)
		lines = append(lines, m.parameterInput.View())
	}
	if m.warning != "" {
		lines = append(lines, catalogWarningStyle.Render(ansi.Wrap(m.warning, inner, "")))
	}
	if m.notice != "" {
		lines = append(lines, catalogDetailStyle.Render(ansi.Wrap(m.notice, inner, "")))
	}
	return section("SELECÇÃO E PESQUISA", strings.Join(lines, "\n"), m.width)
}

// View renders compact rows and reserves a detail line for the active pipeline.
func (m CatalogModel) View() string {
	if m.showDetails {
		p, _ := m.active()
		detail := fmt.Sprintf("Pipeline: %s\n\nProjecto: %s\nRepositório: %s\nPasta: %s\nTags: %s\n\n%s", p.Name, p.Project, p.RepoName, p.Folder, strings.Join(p.Tags, ", "), p.MetadataWarning)
		return section("DETALHE DA PIPELINE", textPage(detail, m.width-4, m.detailScroll, max(1, m.height-6))+"\n\n↑/↓ percorrer · esc voltar", m.width)
	}
	if m.input == inputParameterForm {
		pipeline, _ := m.active()
		return section("CONFIGURAR PARÂMETROS", m.editor.view(max(1, m.width-4), max(1, m.height-2), pipeline.Name), m.width)
	}
	lines := []string{m.selectionView()}

	start, end := m.displayRange()
	inner := max(1, m.width-4)
	widths := []int{4, 4, 5, 10, max(1, inner-35)}
	headers := []string{"SEL", "MODO", "ID", "TIPO", "PIPELINE"}
	if m.hasProjectColumn() && m.width >= 70 {
		widths = []int{4, 4, 5, 15, 10, max(1, inner-53)}
		headers = []string{"SEL", "MODO", "ID", "PROJECTO", "TIPO", "PIPELINE"}
	} else if m.hasProjectColumn() {
		widths = []int{4, 4, 5, 12, max(1, inner-34)}
		headers = []string{"SEL", "MODO", "ID", "PROJECTO", "PIPELINE"}
	} else if m.width < 70 {
		widths = []int{4, 4, 5, max(1, inner-22)}
		headers = []string{"SEL", "MODO", "ID", "PIPELINE"}
	}
	rows := []string{catalogHeaderStyle.Width(inner).Render(tableCells(widths, headers...))}
	if len(m.visible) == 0 {
		rows = append(rows, catalogDetailStyle.Render("Nenhuma pipeline encontrada."))
	}
	for index := start; index < end; index++ {
		pipeline := m.visible[index]
		key := domainrunner.PipelineKey(pipeline)
		marker := " [ ]"
		mode := "-"
		if selectedMode, ok := m.selected[key]; ok {
			marker = " [x]"
			mode = string(selectedMode)
		}
		if index == m.cursor {
			marker = ">" + marker[1:]
		}
		values := []string{marker, mode, strconv.Itoa(pipeline.ID), pipeline.Type(), pipeline.Name}
		if m.hasProjectColumn() && m.width >= 70 {
			values = []string{marker, mode, strconv.Itoa(pipeline.ID), pipeline.Project, pipeline.Type(), pipeline.Name}
		} else if m.hasProjectColumn() {
			values = []string{marker, mode, strconv.Itoa(pipeline.ID), pipeline.Project, pipeline.Name}
		} else if m.width < 70 {
			values = []string{marker, mode, strconv.Itoa(pipeline.ID), pipeline.Name}
		}
		var accents map[int]lipgloss.Style
		if _, selected := m.selected[key]; selected {
			accents = map[int]lipgloss.Style{0: brandLimeStyle, 1: modeStyle(mode)}
		}
		rows = append(rows, renderTableRow(widths, values, index, index == m.cursor, accents))
	}
	for len(rows) < min(3, m.catalogCapacity())+1 {
		rows = append(rows, "")
	}
	compactDetail := "Nenhuma pipeline activa. Altera a pesquisa."
	detail := "Nenhuma pipeline activa.\nAltere o filtro para ver resultados.\nA selecção anterior mantém-se."
	if pipeline, ok := m.active(); ok {
		capability := "PLAN indisponível: sem contrato validado"
		capabilityStyle := catalogDetailStyle
		if pipeline.PlanContract != nil {
			capability = "PLAN disponível por contrato"
			capabilityStyle = planStyle
		}
		if pipeline.MetadataWarning != "" {
			capability = "Metadados incompletos · d para consultar aviso"
			capabilityStyle = catalogWarningStyle
		}
		compactDetail = capabilityStyle.Render(capability)
		left := (inner - 3) / 2
		detail = tableCells([]int{left, inner - 3 - left}, metadata("Repositório", pipeline.RepoName), metadata("Pasta", pipeline.Folder)) + "\n" +
			metadata("Tags", strings.Join(pipeline.Tags, ", ")) + "\n" +
			capabilityStyle.Render(capability) + catalogDetailStyle.Render(" · "+quantity(len(m.parameters[domainrunner.PipelineKey(pipeline)]), "parâmetro", "parâmetros"))
	}
	compact := m.height < 28 && len(m.visible) > 3
	if compact {
		rows = append(rows, borderStyle.Render(strings.Repeat("─", inner)), catalogDetailStyle.Render(truncateWidth("d detalhe completo · "+compactDetail, inner)))
	}
	lines = append(lines, section(fmt.Sprintf("PIPELINES %d–%d / %d", min(start+1, len(m.visible)), end, len(m.visible)), strings.Join(rows, "\n"), m.width))
	if !compact {
		lines = append(lines, section("DETALHE DA PIPELINE ACTIVA", detail, m.width))
	}
	lines = append(lines, section("ACÇÕES E AJUDA · a ou ? abre o menu", m.helpView(), m.width))
	if m.height >= 30 {
		return strings.Join(lines, "\n\n")
	}
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
	detailHeight, gaps := 5, 3
	if m.height < 28 && len(m.visible) > 3 {
		detailHeight, gaps = 2, 2
	}
	// Reserve the app's two-line context and measure controls, including wrapped alerts.
	available := m.height - 2 - lipgloss.Height(m.selectionView()) - 3 - detailHeight -
		lipgloss.Height(section("ACÇÕES E AJUDA", m.helpView(), m.width))
	if m.height >= 30 {
		available -= gaps
	}
	return max(1, available)
}

func (m CatalogModel) helpView() string {
	if m.input == inputSearch {
		return shortcutBar(max(1, m.width-4), "enter terminar pesquisa", "ctrl+u limpar", "esc manter filtro")
	}
	if m.input == inputBranch {
		return shortcutBar(max(1, m.width-4), "enter aplicar a todas", "esc cancelar edição")
	}
	if m.input != inputNone {
		return shortcutBar(max(1, m.width-4), "enter guardar e voltar à lista", "esc cancelar edição")
	}
	primary := "espaço seleccionar"
	if len(m.selected) > 0 {
		primary = "enter rever " + quantity(len(m.selected), "pipeline", "pipelines")
	}
	return shortcutBar(max(1, m.width-4), primary) + "\n" + shortcutBar(max(1, m.width-4), "/ procurar", "a/? acções e ajuda", ":q sair")
}

func (m CatalogModel) nextStep() string {
	visibleSelected := 0
	for _, p := range m.visible {
		if _, ok := m.selected[domainrunner.PipelineKey(p)]; ok {
			visibleSelected++
		}
	}
	if len(m.selected) > 0 {
		next := "Enter revê"
		if m.input == inputSearch {
			next = "Enter termina pesquisa"
		}
		return fmt.Sprintf("%s · %s pelo filtro · %s", quantity(len(m.selected), "seleccionada", "seleccionadas"), quantity(len(m.selected)-visibleSelected, "oculta", "ocultas"), next)
	}
	if m.input == inputSearch {
		return "A procurar pipelines. Enter termina a pesquisa."
	}
	if len(m.visible) == 0 {
		return "Sem resultados. Altera a pesquisa com /."
	}
	return "Selecciona com espaço. a/? abre acções, perfis e histórico."
}

func (m CatalogModel) pipelineRow(pipeline azdo.Pipeline, active bool) string {
	marker := " "
	check := "[ ]"
	if active {
		marker = ">"
	}
	mode := "-"
	if selectedMode, selected := m.selected[domainrunner.PipelineKey(pipeline)]; selected {
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
	if pipeline.Project != "" {
		detail = fmt.Sprintf("    projecto: %s | %s", pipeline.Project, strings.TrimSpace(strings.TrimPrefix(detail, "    ")))
	}
	if pipeline.MetadataWarning != "" {
		detail = "    ⚠ " + pipeline.MetadataWarning + " | " + detail
	}
	return truncateWidth(detail, m.width)
}

func (m CatalogModel) hasProjectColumn() bool {
	return hasMultiplePipelineProjects(m.pipelines)
}

func hasMultiplePipelineProjects(pipelines []azdo.Pipeline) bool {
	projects := map[string]bool{}
	for _, pipeline := range pipelines {
		if pipeline.Project != "" {
			projects[strings.ToLower(pipeline.Project)] = true
		}
	}
	return len(projects) > 1
}

func pipelineDisplayName(pipeline azdo.Pipeline, includeProject bool) string {
	if includeProject && pipeline.Project != "" {
		return pipeline.Project + " / " + pipeline.Name
	}
	return pipeline.Name
}

func truncateWidth(value string, width int) string {
	if width <= 0 {
		return value
	}
	return ansi.Truncate(value, width, "…")
}

func horizontalWindow(value string, offset, width int) string {
	runes := []rune(value)
	if offset >= len(runes) {
		return ""
	}
	return truncateWidth(string(runes[offset:]), width)
}
