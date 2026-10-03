package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
)

type branchLoaded struct {
	token    uint64
	repos    []azdo.Repository
	branches []azdo.Branch
	entries  []branchEntry
	err      error
}
type branchReviewed struct {
	token    uint64
	branches []azdo.Branch
	err      error
}
type branchDeleted struct {
	token   uint64
	results []string
}
type branchExitMsg struct{}

// BranchModel keeps selection, review and destructive confirmation in a separate screen.
type BranchModel struct {
	client                        azdo.Client
	api                           azdo.BranchClient
	project                       string
	repos                         []azdo.Repository
	repo                          azdo.Repository
	branches                      []azdo.Branch
	entries                       []branchEntry
	selected                      map[string]bool
	reviewed                      []azdo.Branch
	cursor, width, height         int
	detailOffset                  int
	stage                         string
	busy                          bool
	showHelp                      bool
	err                           string
	filter, creator, confirmation textinput.Model
	input                         string
	results                       []string
	demo                          bool
	returnToCatalog               bool
	originFilter                  map[branchOrigin]bool
	workingDirectory              string
	command                       commandModel
	opCtx                         context.Context
	cancel                        context.CancelFunc
	generation                    uint64
}

func (m *BranchModel) startOperation() uint64 {
	if m.cancel != nil {
		m.cancel()
	}
	m.generation++
	m.opCtx, m.cancel = context.WithCancel(context.Background())
	return m.generation
}

func NewBranchModel(client azdo.Client, project string) BranchModel {
	filter, creator, confirmation := textinput.New(), textinput.New(), textinput.New()
	filter.Prompt = "Branch: "
	creator.Prompt = "Criador: "
	confirmation.Prompt = "Confirmação: "
	filter.CharLimit = 256
	creator.CharLimit = 256
	confirmation.CharLimit = 8
	api, _ := client.(azdo.BranchClient)
	m := BranchModel{client: client, api: api, project: project, selected: map[string]bool{}, originFilter: allBranchOrigins(), width: 100, height: 32, stage: "repos", filter: filter, creator: creator, confirmation: confirmation, busy: true}
	m.startOperation()
	return m
}

func NewBranchDemo() BranchModel {
	m := NewBranchModel(nil, "example-project")
	m.demo = true
	m.busy = false
	m.stage = "list"
	m.repo = azdo.Repository{ID: "demo", Name: "sample-repo", DefaultBranch: "refs/heads/main"}
	for _, name := range []string{"main", "feat/catalog", "fix/layout"} {
		b := azdo.Branch{Name: "refs/heads/" + name, ObjectID: strings.Repeat("a", 40)}
		b.Creator.DisplayName = "Example User"
		b.Creator.UniqueName = "user@example.com"
		if name == "main" {
			b.Blocked = "branch principal protegida"
		}
		m.branches = append(m.branches, b)
		m.entries = append(m.entries, remoteBranchEntry(b))
	}
	return m
}

func (m BranchModel) Init() tea.Cmd {
	if m.demo {
		return nil
	}
	token := m.generation
	if m.client == nil {
		return func() tea.Msg {
			return branchLoaded{token: token, err: fmt.Errorf("cliente Azure DevOps indisponível")}
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.opCtx, 30*time.Second)
		defer cancel()
		repos, err := m.client.ListRepositories(ctx, m.project)
		return branchLoaded{token: token, repos: repos, err: err}
	}
}

func allBranchOrigins() map[branchOrigin]bool {
	return map[branchOrigin]bool{
		branchOriginRemote:   true,
		branchOriginLocal:    true,
		branchOriginWorktree: true,
	}
}

func (m BranchModel) displayEntries() []branchEntry {
	if len(m.entries) > 0 {
		return m.entries
	}
	entries := make([]branchEntry, 0, len(m.branches))
	for _, branch := range m.branches {
		entries = append(entries, remoteBranchEntry(branch))
	}
	return entries
}

func (m BranchModel) visibleEntries() []branchEntry {
	var rows []branchEntry
	for _, entry := range m.displayEntries() {
		if !m.originFilter[entry.origin] {
			continue
		}
		if entry.branch().Matches(m.filter.Value(), m.creator.Value()) {
			rows = append(rows, entry)
		}
	}
	return rows
}

func (m BranchModel) visible() []azdo.Branch {
	entries := m.visibleEntries()
	rows := make([]azdo.Branch, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, entry.branch())
	}
	return rows
}

func (m BranchModel) selectedRemoteBranches() []azdo.Branch {
	var selected []azdo.Branch
	for _, entry := range m.displayEntries() {
		if entry.selectable() && m.isSelected(entry) {
			selected = append(selected, entry.branch())
		}
	}
	return selected
}

func (m BranchModel) isSelected(entry branchEntry) bool {
	return m.selected[entry.key()] || (entry.selectable() && m.selected[entry.branch().Name])
}

func (m *BranchModel) toggleOrigin(origin branchOrigin) {
	if m.originFilter == nil {
		m.originFilter = allBranchOrigins()
	}
	m.originFilter[origin] = !m.originFilter[origin]
	if !m.originFilter[branchOriginRemote] && !m.originFilter[branchOriginLocal] && !m.originFilter[branchOriginWorktree] {
		m.originFilter = allBranchOrigins()
	}
	m.cursor = 0
}

func (m BranchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && m.command.active {
		action, cmd := m.command.update(key)
		if action == commandQuit {
			if m.busy {
				if m.cancel != nil {
					m.cancel()
				}
				m.command.close()
				m.err = "Cancelamento pedido; aguarda o resultado. Não desfaz pedidos aceites."
				return m, nil
			}
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		if action == commandBack {
			return m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		}
		return m, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok && m.input == "" && !m.confirmation.Focused() {
		if cmd, opened := m.command.startFromKey(key, m.width); opened {
			return m, cmd
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok && m.showHelp && !m.busy && (key.Type == tea.KeyCtrlC || key.Type == tea.KeyCtrlD) {
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	if key, ok := msg.(tea.KeyMsg); ok && !m.busy && m.input == "" {
		if key.String() == "?" {
			m.showHelp = !m.showHelp
			return m, nil
		}
		if m.showHelp {
			if key.Type == tea.KeyEsc {
				m.showHelp = false
			}
			return m, nil
		}
	}
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.height = v.Height
		return m, nil
	case branchLoaded:
		if v.token != m.generation {
			return m, nil
		}
		m.busy = false
		if v.err != nil {
			m.branches = nil
			m.entries = nil
			m.err = v.err.Error()
			return m, nil
		}
		m.err = ""
		m.cursor = 0
		if m.stage == "repos" {
			m.repos = v.repos
		} else {
			m.branches = v.branches
			m.entries = v.entries
		}
		return m, nil
	case branchReviewed:
		if v.token != m.generation {
			return m, nil
		}
		m.busy = false
		m.reviewed = v.branches
		if v.err != nil {
			m.err = v.err.Error()
			m.stage = "list"
			return m, nil
		}
		m.stage = "review"
		m.cursor = 0
		m.confirmation.SetValue("")
		m.confirmation.Blur()
		if m.demo {
			return m, nil
		}
		return m, m.confirmation.Focus()
	case branchDeleted:
		if v.token != m.generation {
			return m, nil
		}
		m.confirmation.Blur()
		m.busy = false
		m.stage = "results"
		m.results = v.results
		m.cursor = 0
		m.selected = map[string]bool{}
		m.reviewed = nil
		return m, nil
	case tea.KeyMsg:
		if m.busy {
			if v.String() == "esc" || v.String() == "ctrl+c" {
				if m.cancel != nil {
					m.cancel()
				}
				m.err = "Cancelamento pedido; aguarda o resultado. Não desfaz pedidos aceites."
			}
			return m, nil
		}
		if v.String() == "ctrl+c" || v.String() == "ctrl+d" {
			return m, tea.Quit
		}
		if m.input == "" {
			if v.String() == "left" {
				m.detailOffset = max(0, m.detailOffset-1)
				return m, nil
			}
			if v.String() == "right" {
				m.detailOffset++
				return m, nil
			}
			if v.String() == "up" || v.String() == "down" || v.String() == "j" || v.String() == "k" {
				m.detailOffset = 0
			}
		}
		if m.input != "" {
			if v.String() == "esc" || v.String() == "enter" {
				m.input = ""
				m.filter.Blur()
				m.creator.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			if m.input == "creator" {
				m.creator, cmd = m.creator.Update(v)
			} else {
				m.filter, cmd = m.filter.Update(v)
			}
			m.cursor = 0
			return m, cmd
		}
		if m.stage == "review" {
			switch v.String() {
			case "esc":
				m.confirmation.Blur()
				m.stage = "list"
				m.reviewed = nil
				m.confirmation.SetValue("")
				m.err = ""
				return m, nil
			case "up":
				m.cursor = max(0, m.cursor-1)
				return m, nil
			case "down":
				m.cursor = min(max(0, len(m.reviewed)-1), m.cursor+1)
				return m, nil
			case "enter":
				if m.demo {
					m.err = "Demo: eliminação indisponível."
					return m, nil
				}
				if m.api == nil {
					m.err = "Este cliente não suporta gestão de branches."
					return m, nil
				}
				if len(m.reviewed) == 0 || m.confirmation.Value() != "ELIMINAR" {
					m.err = "Escreve ELIMINAR exactamente para confirmar."
					return m, nil
				}
				for _, b := range m.reviewed {
					if b.Blocked != "" {
						m.err = "Retira as branches bloqueadas da selecção."
						return m, nil
					}
				}
				m.busy = true
				m.err = ""
				m.confirmation.Blur()
				token := m.startOperation()
				return m, func() tea.Msg {
					var results []string
					for _, b := range m.reviewed {
						if m.opCtx.Err() != nil {
							results = append(results, b.Name+" · não iniciada: lote cancelado")
							continue
						}
						ctx, cancel := context.WithTimeout(m.opCtx, 45*time.Second)
						err := m.api.DeleteBranch(ctx, m.project, m.repo.ID, b)
						cancel()
						status := "eliminada"
						if err != nil {
							status = err.Error()
						}
						results = append(results, b.Name+" · SHA "+b.ObjectID+" · "+status)
					}
					return branchDeleted{token: token, results: results}
				}
			}
			var cmd tea.Cmd
			m.confirmation, cmd = m.confirmation.Update(v)
			return m, cmd
		}
		switch v.String() {
		case "esc":
			if m.cancel != nil {
				m.cancel()
			}
			if m.stage == "review" {
				m.stage = "list"
				m.reviewed = nil
				m.confirmation.SetValue("")
				m.err = ""
				return m, nil
			}
			if m.stage == "list" && len(m.repos) > 0 {
				m.stage = "repos"
				m.cursor = 0
				m.selected = map[string]bool{}
				m.err = ""
				return m, nil
			}
			if m.stage == "results" {
				m.stage = "list"
				m.cursor = 0
				m.err = ""
				return m, nil
			}
			if m.returnToCatalog {
				return m, func() tea.Msg { return branchExitMsg{} }
			}
			return m, func() tea.Msg { return branchExitMsg{} }
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			count := len(m.visible())
			if m.stage == "repos" {
				count = len(m.repos)
			}
			if m.stage == "results" {
				count = len(m.results)
			}
			m.cursor = min(max(0, count-1), m.cursor+1)
		case "/":
			if m.stage == "list" {
				m.input = "filter"
				return m, m.filter.Focus()
			}
		case "u":
			if m.stage == "list" {
				m.input = "creator"
				return m, m.creator.Focus()
			}
		case "c":
			if m.stage == "list" {
				m.filter.SetValue("")
				m.creator.SetValue("")
				m.originFilter = allBranchOrigins()
				m.cursor = 0
			}
		case "R":
			if m.stage == "list" {
				m.toggleOrigin(branchOriginRemote)
			}
		case "L":
			if m.stage == "list" {
				m.toggleOrigin(branchOriginLocal)
			}
		case "W":
			if m.stage == "list" {
				m.toggleOrigin(branchOriginWorktree)
			}
		case "b":
			if !m.demo {
				m.startOperation()
				m.stage = "repos"
				m.cursor = 0
				m.selected = map[string]bool{}
				m.err = ""
			}
		case "r":
			if m.stage == "repos" {
				m.busy = true
				m.err = ""
				m.startOperation()
				return m, m.Init()
			}
			if m.stage == "list" || m.stage == "results" {
				return m.load()
			}
		case " ":
			if m.stage == "list" {
				rows := m.visibleEntries()
				if m.cursor < len(rows) {
					entry := rows[m.cursor]
					if !entry.selectable() {
						m.err = "Branches locais e de worktree são só de leitura; selecciona uma branch remota."
						return m, nil
					}
					key := entry.key()
					m.selected[key] = !m.selected[key]
					if !m.selected[key] {
						delete(m.selected, key)
					}
				}
			}
		case "a":
			if m.stage == "list" {
				for _, entry := range m.visibleEntries() {
					if entry.selectable() {
						m.selected[entry.key()] = true
					}
				}
			}
		case "enter":
			if m.stage == "repos" && m.cursor < len(m.repos) {
				m.repo = m.repos[m.cursor]
				m.filter.SetValue("")
				m.creator.SetValue("")
				m.originFilter = allBranchOrigins()
				return m.load()
			}
			if m.stage == "list" {
				selected := m.selectedRemoteBranches()
				if len(selected) == 0 {
					m.err = "Selecciona branches com espaço."
					return m, nil
				}
				if m.api == nil && !m.demo {
					m.err = "Este cliente não suporta gestão de branches."
					return m, nil
				}
				m.busy = true
				m.err = ""
				token := m.startOperation()
				return m, func() tea.Msg {
					var reviewed []azdo.Branch
					for _, b := range selected {
						if m.demo {
							reviewed = append(reviewed, b)
							continue
						}
						ctx, cancel := context.WithTimeout(m.opCtx, 30*time.Second)
						r, err := m.api.ReviewBranch(ctx, m.project, m.repo.ID, b)
						cancel()
						if err != nil {
							return branchReviewed{token: token, err: err}
						}
						reviewed = append(reviewed, r)
					}
					return branchReviewed{token: token, branches: reviewed}
				}
			}
		}
	}
	return m, nil
}

func (m BranchModel) load() (BranchModel, tea.Cmd) {
	m.stage = "list"
	m.cursor = 0
	m.selected = map[string]bool{}
	m.reviewed = nil
	m.err = ""
	if m.demo {
		return m, nil
	}
	m.branches = nil
	m.entries = nil
	if m.api == nil {
		m.err = "Cliente sem suporte de branches."
		return m, nil
	}
	m.busy = true
	token := m.startOperation()
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.opCtx, 45*time.Second)
		defer cancel()
		branches, err := m.api.ListBranches(ctx, m.project, m.repo.ID)
		if err != nil {
			return branchLoaded{token: token, err: err}
		}
		entries := make([]branchEntry, 0, len(branches))
		for _, branch := range branches {
			entries = append(entries, remoteBranchEntry(branch))
		}
		local := discoverLocalBranches(ctx, m.workingDirectory)
		if repositoriesMatch(m.repo.RemoteURL, local.remoteURL...) {
			entries = append(entries, local.entries...)
		}
		return branchLoaded{token: token, branches: branches, entries: entries}
	}
}

func branchTableLayout(width int, stage string) (int, []int, []string) {
	inner := max(1, width-4)
	switch stage {
	case "list":
		if inner < 48 {
			return inner, []int{4, max(1, inner-7)}, []string{"SEL", "BRANCH / ORIGEM"}
		}
		originWidth := min(9, max(7, inner/6))
		creatorWidth := min(20, max(12, inner/4))
		branchWidth := inner - 13 - originWidth - creatorWidth
		if branchWidth < 14 {
			creatorWidth = max(10, creatorWidth-(14-branchWidth))
			branchWidth = inner - 13 - originWidth - creatorWidth
		}
		return inner, []int{4, originWidth, branchWidth, creatorWidth}, []string{"SEL", "ORIGEM", "BRANCH", "CRIADOR"}
	case "review":
		statusWidth := min(18, max(8, inner/3))
		return inner, []int{max(1, inner-3-statusWidth), statusWidth}, []string{"BRANCH", "ESTADO"}
	case "repos":
		if inner < 27 {
			return inner, []int{max(1, inner-3), 3}, []string{"REPOSITÓRIO", "DEFAULT"}
		}
		defaultWidth := min(24, max(14, inner/3))
		return inner, []int{inner - 3 - defaultWidth, defaultWidth}, []string{"REPOSITÓRIO", "BRANCH DEFAULT"}
	case "results":
		if inner < 34 {
			return inner, []int{max(1, inner-3), 3}, []string{"BRANCH", "ESTADO"}
		}
		statusWidth := min(32, max(16, inner/3))
		return inner, []int{inner - 3 - statusWidth, statusWidth}, []string{"BRANCH", "ESTADO"}
	default:
		return inner, []int{inner}, []string{"RESULTADO"}
	}
}

func renderBranchTable(inner int, widths []int, headers []string, rows [][]string, active int, selected map[int]bool) string {
	lines := []string{catalogHeaderStyle.Width(inner).Render(tableCells(widths, headers...))}
	if len(rows) == 0 {
		return strings.Join(append(lines, catalogDetailStyle.Render("Sem resultados.")), "\n")
	}
	for index, values := range rows {
		if index == active && len(values) > 0 {
			values = append([]string(nil), values...)
			values[0] = ">" + values[0]
		}
		var accents map[int]lipgloss.Style
		if selected[index] {
			accents = map[int]lipgloss.Style{0: brandLimeStyle}
		}
		lines = append(lines, renderTableRow(widths, values, index, index == active, accents))
	}
	return strings.Join(lines, "\n")
}

func branchCreator(branch azdo.Branch) string {
	creator := branch.Creator.UniqueName
	if creator == "" {
		creator = branch.Creator.DisplayName
	}
	if creator == "" {
		return "criador desconhecido"
	}
	return creator
}

func branchState(branch azdo.Branch) string {
	if branch.Blocked != "" {
		return "BLOQUEADA"
	}
	if branch.IsLocked {
		return "BLOQUEADA"
	}
	return "disponível"
}

func branchResultCells(result string) []string {
	parts := strings.SplitN(result, " · ", 3)
	if len(parts) == 3 {
		return []string{strings.TrimPrefix(parts[0], "refs/heads/"), parts[2]}
	}
	return []string{result}
}

func (m BranchModel) breadcrumb() string {
	parts := []string{"AZPIPE", m.project, "Repositórios"}
	if m.repo.Name != "" {
		parts = append(parts, m.repo.Name)
	}
	switch m.stage {
	case "list":
		parts = append(parts, "Branches")
	case "review":
		parts = append(parts, "Revisão")
	case "results":
		parts = append(parts, "Resultados")
	}
	return catalogDetailStyle.Render(strings.Join(parts, " / "))
}

func (m BranchModel) originFiltersLabel() string {
	labels := []string{}
	for _, origin := range []branchOrigin{branchOriginRemote, branchOriginLocal, branchOriginWorktree} {
		if m.originFilter[origin] {
			labels = append(labels, string(origin))
		}
	}
	if len(labels) == 0 {
		return "nenhuma"
	}
	return strings.Join(labels, ", ")
}

func (m BranchModel) View() string {
	frameWidth := max(10, m.width)
	if m.showHelp {
		view := section("BRANCHES · AJUDA", "/   Filtrar pelo nome\nu   Filtrar pelo criador\nR   Mostrar/ocultar remotas\nL   Mostrar/ocultar locais\nW   Mostrar/ocultar worktrees\na   Seleccionar todas as remotas visíveis\nc   Limpar filtros\n\nr   Actualizar\nb   Escolher repositório\n←/→ Percorrer detalhe e erros\n\nEsc Voltar\n:q  Sair\n?   Fechar ajuda", frameWidth)
		if m.command.active {
			view += "\n" + m.command.view(frameWidth)
		}
		return view
	}
	inner, widths, headers := branchTableLayout(frameWidth, m.stage)
	prefix := wordmarkStyle.Render("AZPIPE") + "  " + catalogTitleStyle.Render("GESTÃO DE BRANCHES") + "\n" + truncateWidth(m.breadcrumb(), frameWidth) + "\n"
	controlsTitle := "CONTEXTO"
	var controls []string
	var tableRows [][]string
	var total int
	var detail string
	tableTitle := "RESULTADOS"
	help := shortcutBar(inner, "↑/↓ escolher", "enter abrir", "? ajuda", "esc anterior", ":q sair")
	switch m.stage {
	case "repos":
		controlsTitle = "ESCOLHER REPOSITÓRIO"
		controls = append(controls, "Escolhe um repositório para abrir as branches.")
		tableTitle = "REPOSITÓRIOS"
		total = len(m.repos)
		for _, repo := range m.repos {
			tableRows = append(tableRows, []string{repo.Name, repo.DefaultBranch})
		}
		if m.cursor < total {
			repo := m.repos[m.cursor]
			detail = fmt.Sprintf("Repositório: %s\nBranch default: %s", repo.Name, repo.DefaultBranch)
		}
	case "list":
		controlsTitle = "SELECÇÃO E FILTROS"
		m.filter.Width = max(8, inner-ansi.StringWidth(m.filter.Prompt)-1)
		m.creator.Width = max(8, inner-ansi.StringWidth(m.creator.Prompt)-1)
		controls = append(controls, "", catalogDetailStyle.Render("Origens · R/L/W: "+m.originFiltersLabel()), m.filter.View(), m.creator.View())
		tableTitle = "BRANCHES"
		visible := m.visibleEntries()
		total = len(visible)
		for _, entry := range visible {
			b := entry.branch()
			mark := "[ ]"
			if m.isSelected(entry) {
				mark = "[x]"
			}
			if !entry.selectable() {
				mark = "[-]"
			}
			values := []string{mark, entry.originLabel(), strings.TrimPrefix(b.Name, "refs/heads/"), branchCreator(b)}
			if len(widths) == 2 {
				values = []string{values[0], values[1] + " · " + values[2]}
			}
			tableRows = append(tableRows, values)
		}
		if m.cursor < total {
			entry := visible[m.cursor]
			b := entry.branch()
			detail = fmt.Sprintf("Estado: %s\nBranch: %s\nOrigem: %s\nLocal: %s\nCriador: %s\nSHA: %s", branchState(b), b.Name, entry.originLabel(), entry.locationLabel(), branchCreator(b), b.ObjectID)
		}
		help = shortcutBar(inner, "espaço seleccionar remota", "enter rever") + "\n" + shortcutBar(inner, "/ nome", "u criador", "? ajuda", "esc anterior", ":q sair")
	case "review":
		controlsTitle = "REVER ELIMINAÇÃO · REMOTAS"
		controls = append(controls, "Confirma o repositório, os nomes e os SHAs.", "Isto não confirma que os commits já foram integrados.")
		tableTitle = "REVISÃO"
		total = len(m.reviewed)
		for _, b := range m.reviewed {
			status := "Sem bloqueios"
			if b.Blocked != "" {
				status = "BLOQUEADA: " + b.Blocked
			}
			values := []string{strings.TrimPrefix(b.Name, "refs/heads/"), b.ObjectID, status}
			if len(widths) == 2 {
				values = []string{strings.TrimPrefix(b.Name, "refs/heads/"), status}
			}
			tableRows = append(tableRows, values)
		}
		if m.cursor < total {
			b := m.reviewed[m.cursor]
			detail = fmt.Sprintf("SHA: %s\nBranch: %s\nEstado: %s\nCriador: %s", b.ObjectID, b.Name, branchState(b), branchCreator(b))
		}
		help = shortcutBar(inner, "↑/↓ rever lista", "esc voltar", "enter confirmar")
	case "results":
		controlsTitle = "RESULTADO DO LOTE"
		controls = append(controls, "Sem repetição automática.")
		tableTitle = "RESULTADOS"
		total = len(m.results)
		for _, result := range m.results {
			tableRows = append(tableRows, branchResultCells(result))
		}
		if m.cursor < total {
			detail = m.results[m.cursor]
		}
		help = shortcutBar(inner, "↑/↓ escolher", "r actualizar", "b repositórios", "esc anterior", ":q sair")
	}
	hidden := 0
	if m.stage == "list" {
		visibleSelected := 0
		for _, b := range m.visible() {
			if m.isSelected(remoteBranchEntry(b)) {
				visibleSelected++
			}
		}
		hidden = len(m.selected) - visibleSelected
		controls[0] = fmt.Sprintf("%s · %s pelo filtro · só remotas", quantity(len(m.selected), "seleccionada", "seleccionadas"), quantity(hidden, "oculta", "ocultas"))
	}
	detailCount := 4
	if m.height < 28 {
		detailCount = 2
	}
	if detail != "" {
		detail = textPage(detail, inner, m.detailOffset, detailCount)
	}
	var footer []string
	if m.stage == "review" {
		if m.demo {
			footer = append(footer, catalogDetailStyle.Render(ansi.Wrap("Exemplo de revisão. Eliminação indisponível.", inner, "")))
		} else {
			footer = append(footer, catalogWarningStyle.Render(ansi.Wrap(fmt.Sprintf("Vai eliminar %s de %s / %s.", quantity(len(m.reviewed), "branch", "branches"), m.project, m.repo.Name), inner, "")), "Escreve ELIMINAR e prime Enter para eliminar.", m.confirmation.View())
		}
	}
	if m.busy {
		footer = append(footer, catalogDetailStyle.Render(ansi.Wrap("A processar… Esc ou :q cancela o restante lote.", inner, "")))
	}
	if m.demo {
		footer = append(footer, catalogDetailStyle.Render("DEMO OFFLINE · não elimina branches"))
	}
	if m.stage == "review" && m.demo {
		help = shortcutBar(inner, "↑/↓ rever lista", "esc voltar à selecção", ":q sair")
	} else if m.input != "" {
		help = shortcutBar(inner, "enter terminar pesquisa", "esc voltar à lista")
	} else if m.stage != "review" && m.returnToCatalog {
		help = strings.ReplaceAll(help, "anterior", "catálogo")
	}
	footer = append(footer, help)
	compact := m.height < 28 && m.stage != "review"
	render := func(start, end int) string {
		selectedRows := map[int]bool{}
		if m.stage == "list" {
			for index := start; index < end; index++ {
				selectedRows[index-start] = m.isSelected(m.visibleEntries()[index])
			}
		}
		table := renderBranchTable(inner, widths, headers, tableRows[start:end], m.cursor-start, selectedRows)
		if compact && detail != "" {
			table += "\n" + borderStyle.Render(strings.Repeat("─", inner)) + "\n" + catalogDetailStyle.Render(detail)
		}
		panels := []string{section(controlsTitle, strings.Join(controls, "\n"), frameWidth), section(fmt.Sprintf("%s %d–%d / %d · ←/→ detalhe", tableTitle, min(start+1, total), end, total), table, frameWidth)}
		if !compact && detail != "" {
			panels = append(panels, section("DETALHE · ←/→ deslocar", detail, frameWidth))
		}
		if m.err != "" {
			panels = append(panels, section("ERRO · ←/→ percorrer", catalogWarningStyle.Render(horizontalWindow(m.err, m.detailOffset*20, inner)), frameWidth))
		}
		panels = append(panels, section("ACÇÕES", strings.Join(footer, "\n"), frameWidth))
		separator := "\n"
		if m.height >= 30 {
			separator = "\n\n"
		}
		view := prefix + strings.Join(panels, separator)
		if m.command.active {
			view += "\n" + m.command.view(frameWidth)
		}
		return view
	}
	// One measured row fixes the space left for the table; no guessed reservation.
	capacity := max(1, 1+m.height-lipgloss.Height(render(0, min(1, total))))
	start := min(max(0, m.cursor-capacity+1), max(0, total-capacity))
	end := min(total, start+capacity)
	return render(start, end)
}
