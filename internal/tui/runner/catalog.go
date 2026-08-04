package runner

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
)

// CatalogReviewMsg moves the selected pipelines to the review screen.
type CatalogReviewMsg struct {
	Selections []domainrunner.Selection
}

// CatalogModel lets an operator search and select pipelines before review.
type CatalogModel struct {
	pipelines []azdo.Pipeline
	visible   []azdo.Pipeline
	selected  map[int]domainrunner.Mode
	search    textinput.Model
	branch    textinput.Model
	cursor    int
	width     int
	height    int
	input     catalogInput
	warning   string
}

// NewCatalogModel creates a catalog with all pipelines visible and main as branch.
func NewCatalogModel(pipelines []azdo.Pipeline) CatalogModel {
	search := textinput.New()
	search.Prompt = "Procurar: "
	search.Width = 40
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
	return model
}

// Init performs no asynchronous work.
func (m CatalogModel) Init() tea.Cmd {
	return nil
}

// Selected returns the current selections in catalog order.
func (m CatalogModel) Selected() []domainrunner.Selection {
	selections := make([]domainrunner.Selection, 0, len(m.selected))
	branch := strings.TrimSpace(m.branch.Value())
	for _, pipeline := range m.pipelines {
		mode, ok := m.selected[pipeline.ID]
		if !ok {
			continue
		}
		selections = append(selections, domainrunner.Selection{
			Pipeline: pipeline,
			Mode:     mode,
			Branch:   branch,
		})
	}
	return selections
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
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "/":
		m.input = inputSearch
		return m, m.search.Focus()
	case "b":
		m.input = inputBranch
		return m, m.branch.Focus()
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
	case "p":
		if pipeline, ok := m.active(); ok {
			m.selected[pipeline.ID] = domainrunner.ModePlan
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
	var cmd tea.Cmd
	switch m.input {
	case inputSearch:
		m.search, cmd = m.search.Update(msg)
		m.filter()
	case inputBranch:
		m.branch, cmd = m.branch.Update(msg)
	}
	return m, cmd
}

func (m CatalogModel) escape() (tea.Model, tea.Cmd) {
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
	lines := []string{
		catalogTitleStyle.Render("Pipelines"),
		m.search.View(),
		catalogHeaderStyle.Render("SEL MODE    ID TYPE         PIPELINE"),
	}

	start, end := m.displayRange()
	if len(m.visible) == 0 {
		lines = append(lines, catalogDetailStyle.Render("Nenhuma pipeline encontrada."))
	}
	for index := start; index < end; index++ {
		pipeline := m.visible[index]
		row := m.pipelineRow(pipeline, index == m.cursor)
		if index == m.cursor {
			row = catalogActiveStyle.Render(row)
		}
		lines = append(lines, row)
		if index == m.cursor {
			lines = append(lines, catalogDetailStyle.Render(m.pipelineDetail(pipeline)))
		}
	}
	if m.input == inputBranch {
		lines = append(lines, m.branch.View())
	}
	if m.warning != "" {
		lines = append(lines, catalogWarningStyle.Render(m.warning))
	}
	if m.height >= 12 {
		lines = append(lines, catalogFooterStyle.Render("↑/↓ ou j/k navegar • espaço selecionar • p plano • / procurar • b branch • enter rever • q sair"))
	}
	return strings.Join(lines, "\n")
}

func (m CatalogModel) displayRange() (int, int) {
	if len(m.visible) == 0 {
		return 0, 0
	}
	available := m.height - 4 // title, search, header, footer
	if m.warning != "" {
		available--
	}
	if m.input == inputBranch {
		available--
	}
	available = max(2, available)

	start, end, used := m.cursor, m.cursor+1, 2 // active row plus its detail
	for end < len(m.visible) && used+1 <= available {
		end++
		used++
	}
	for start > 0 && used+1 <= available {
		start--
		used++
	}
	return start, end
}

func (m CatalogModel) pipelineRow(pipeline azdo.Pipeline, active bool) string {
	marker := " "
	if active {
		marker = ">"
	}
	mode := "-"
	if selectedMode, selected := m.selected[pipeline.ID]; selected {
		marker = "x"
		mode = string(selectedMode)
	}
	nameWidth := max(8, m.width-37)
	return fmt.Sprintf("%s   %-4s %5d %-12s %s", marker, mode, pipeline.ID, truncateWidth(pipeline.Type(), 12), truncateWidth(pipeline.Name, nameWidth))
}

func (m CatalogModel) pipelineDetail(pipeline azdo.Pipeline) string {
	detail := fmt.Sprintf("    repo: %s | folder: %s | tags: %s", pipeline.RepoName, pipeline.Folder, strings.Join(pipeline.Tags, ", "))
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
