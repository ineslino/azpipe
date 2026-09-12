package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
)

type branchLoaded struct {
	token    uint64
	repos    []azdo.Repository
	branches []azdo.Branch
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
	m := BranchModel{client: client, api: api, project: project, selected: map[string]bool{}, width: 100, height: 32, stage: "repos", filter: filter, creator: creator, confirmation: confirmation, busy: true}
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

func (m BranchModel) visible() []azdo.Branch {
	var rows []azdo.Branch
	for _, b := range m.branches {
		if b.Matches(m.filter.Value(), m.creator.Value()) {
			rows = append(rows, b)
		}
	}
	return rows
}

func (m BranchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && m.showHelp && !m.busy && (key.Type == tea.KeyCtrlC || key.Type == tea.KeyCtrlD || key.String() == "q") {
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
			m.err = v.err.Error()
			return m, nil
		}
		m.err = ""
		m.cursor = 0
		if m.stage == "repos" {
			m.repos = v.repos
		} else {
			m.branches = v.branches
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
		return m, m.confirmation.Focus()
	case branchDeleted:
		if v.token != m.generation {
			return m, nil
		}
		m.busy = false
		m.stage = "results"
		m.results = v.results
		m.cursor = 0
		m.selected = map[string]bool{}
		m.reviewed = nil
		return m, nil
	case tea.KeyMsg:
		if m.busy {
			if v.String() == "esc" || v.String() == "ctrl+c" || v.String() == "q" {
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
		case "q":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "esc":
			if m.cancel != nil {
				m.cancel()
			}
			if m.returnToCatalog {
				return m, func() tea.Msg { return branchExitMsg{} }
			}
			return m, tea.Quit
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
				m.cursor = 0
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
				rows := m.visible()
				if m.cursor < len(rows) {
					name := rows[m.cursor].Name
					m.selected[name] = !m.selected[name]
					if !m.selected[name] {
						delete(m.selected, name)
					}
				}
			}
		case "enter":
			if m.stage == "repos" && m.cursor < len(m.repos) {
				m.repo = m.repos[m.cursor]
				m.filter.SetValue("")
				m.creator.SetValue("")
				return m.load()
			}
			if m.stage == "list" {
				var selected []azdo.Branch
				for _, b := range m.branches {
					if m.selected[b.Name] {
						selected = append(selected, b)
					}
				}
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
		return branchLoaded{token: token, branches: branches, err: err}
	}
}

func branchTableLayout(width int, stage string) (int, []int, []string) {
	inner := max(1, width-4)
	switch stage {
	case "list":
		if inner < 34 {
			return inner, []int{4, max(1, inner-7)}, []string{"SEL", "BRANCH"}
		}
		creatorWidth := min(28, max(10, inner/3))
		branchWidth := inner - 10 - creatorWidth
		if branchWidth < 16 {
			creatorWidth = max(8, inner-10-16)
			branchWidth = inner - 10 - creatorWidth
		}
		return inner, []int{4, branchWidth, creatorWidth}, []string{"SEL", "BRANCH", "CRIADOR"}
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
		row := tableCells(widths, values...)
		if index == active {
			row = catalogActiveStyle.Width(inner).Render(row)
		} else if selected[index] {
			row = catalogDetailStyle.Bold(true).Width(inner).Render(row)
		} else if index%2 == 0 {
			row = stripeStyle.Width(inner).Render(row)
		}
		lines = append(lines, row)
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

func (m BranchModel) View() string {
	frameWidth := max(10, m.width)
	if m.showHelp {
		return section("BRANCHES · AJUDA", "\n/   Filtrar pelo nome\nu   Filtrar pelo criador\nc   Limpar filtros\n\nr   Actualizar\nb   Escolher repositório\n←/→ Percorrer detalhe e erros\n\nEsc Voltar\n?   Fechar ajuda", frameWidth)
	}
	inner, widths, headers := branchTableLayout(frameWidth, m.stage)
	lines := []string{catalogTitleStyle.Render(truncateWidth("BRANCHES · "+m.project+" / "+m.repo.Name, frameWidth)), ""}
	var tableRows [][]string
	var total int
	var detail string
	tableTitle := "RESULTADOS"
	help := "↑/↓ navegar · enter abrir · r actualizar · q sair"
	switch m.stage {
	case "repos":
		lines = append(lines, "Escolhe um repositório", "")
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
		lines = append(lines, "Selecciona com espaço. Enter revê; ainda não elimina.", m.filter.View(), m.creator.View(), "")
		tableTitle = "BRANCHES"
		visible := m.visible()
		total = len(visible)
		for _, b := range visible {
			mark := "[ ]"
			if m.selected[b.Name] {
				mark = "[x]"
			}
			values := []string{mark, strings.TrimPrefix(b.Name, "refs/heads/"), branchCreator(b)}
			if len(widths) == 2 {
				values = values[:2]
			}
			tableRows = append(tableRows, values)
		}
		if m.cursor < total {
			b := visible[m.cursor]
			detail = fmt.Sprintf("Branch: %s\nEstado: %s\nCriador: %s\nSHA: %s", b.Name, branchState(b), branchCreator(b), b.ObjectID)
		}
		help = "espaço seleccionar · enter rever · ? mais acções · q sair"
	case "review":
		lines = append(lines, "REVER ELIMINAÇÃO · branches remotas", "Confirma o repositório, os nomes e os SHAs.", "Isto não confirma que os commits já foram integrados.", "")
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
		help = "↑/↓ rever lista · esc voltar · enter confirmar"
	case "results":
		lines = append(lines, "RESULTADOS · sem repetição automática", "")
		tableTitle = "RESULTADOS"
		total = len(m.results)
		for _, result := range m.results {
			tableRows = append(tableRows, branchResultCells(result))
		}
		if m.cursor < total {
			detail = m.results[m.cursor]
		}
		help = "↑/↓ navegar · r actualizar branches · b repos · q sair"
	}
	reserve := 16
	if m.stage == "review" && !m.demo {
		reserve += 2
	}
	if m.err != "" {
		reserve += 3
	}
	capacity := max(1, m.height-len(lines)-reserve)
	if m.height >= 30 {
		capacity = max(1, capacity/2)
	}
	start := max(0, m.cursor-capacity+1)
	if start > total {
		start = max(0, total-capacity)
	}
	end := min(total, start+capacity)
	selectedRows := map[int]bool{}
	if m.stage == "list" {
		for index := start; index < end; index++ {
			if m.selected[m.visible()[index].Name] {
				selectedRows[index-start] = true
			}
		}
	}
	table := renderBranchTable(inner, widths, headers, tableRows[start:end], m.cursor-start, selectedRows)
	if m.height >= 30 {
		rows := strings.Split(table, "\n")
		spaced := []string{rows[0], ""}
		for _, row := range rows[1:] {
			spaced = append(spaced, row, "")
		}
		table = strings.Join(spaced, "\n")
	}
	if total == 0 {
		table = renderBranchTable(inner, widths, headers, nil, -1, nil)
	}
	lines = append(lines, strings.Split(section(fmt.Sprintf("%s %d–%d / %d", tableTitle, min(start+1, total), end, total), table, frameWidth), "\n")...)
	hidden := 0
	if m.stage == "list" {
		visibleSelected := 0
		for _, b := range m.visible() {
			if m.selected[b.Name] {
				visibleSelected++
			}
		}
		hidden = len(m.selected) - visibleSelected
	}
	lines = append(lines, "", fmt.Sprintf("%d/%d · %s (%s pelo filtro)", min(m.cursor+1, total), total, quantity(len(m.selected), "seleccionada", "seleccionadas"), quantity(hidden, "oculta", "ocultas")))
	if detail != "" {
		wrapped := strings.Split(ansi.Wrap(detail, max(1, frameWidth-8), ""), "\n")
		offset := min(m.detailOffset, max(0, len(wrapped)-4))
		wrapped = wrapped[offset:min(len(wrapped), offset+4)]
		lines = append(lines, strings.Split(section("DETALHE · ←/→ deslocar", strings.Join(wrapped, "\n"), frameWidth), "\n")...)
	}
	if m.stage == "review" {
		if m.demo {
			lines = append(lines, "Exemplo de revisão: Esc volta à selecção. Eliminação indisponível.")
		} else {
			lines = append(lines, fmt.Sprintf("Vai eliminar %s de %s / %s.", quantity(len(m.reviewed), "branch", "branches"), m.project, m.repo.Name), "Escreve ELIMINAR e prime Enter para eliminar as branches remotas.", m.confirmation.View())
		}
	}
	if m.busy {
		lines = append(lines, "A processar… Esc cancela o restante lote.")
	}
	if m.err != "" {
		lines = append(lines, "", catalogWarningStyle.Render(horizontalWindow("Erro: "+m.err, m.detailOffset*20, inner)), "←/→ percorrer erro completo")
	}
	if m.demo {
		lines = append(lines, "DEMO OFFLINE · não elimina branches")
	}
	if m.stage == "review" && m.demo {
		help = "↑/↓ rever lista · esc voltar à selecção · ctrl+c sair"
	} else if m.input != "" {
		help = "A filtrar branches · enter terminar pesquisa · esc voltar à lista"
	} else if m.stage != "review" && m.returnToCatalog {
		help += " · esc voltar ao catálogo"
	}
	lines = append(lines, strings.Split(ansi.Wrap(help, inner, ""), "\n")...)
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, frameWidth, "…")
	}
	return strings.Join(lines, "\n")
}
