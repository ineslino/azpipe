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
		case "q", "esc":
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

func (m BranchModel) View() string {
	width := max(10, m.width-4)
	lines := []string{catalogTitleStyle.Render("BRANCHES · " + m.project + " / " + m.repo.Name), ""}
	var rows []string
	help := "↑/↓ navegar · enter abrir · r actualizar · q sair"
	switch m.stage {
	case "repos":
		lines = append(lines, "Escolhe um repositório", "")
		for _, r := range m.repos {
			rows = append(rows, r.Name)
		}
	case "list":
		lines = append(lines, m.filter.View(), m.creator.View(), "", catalogHeaderStyle.Render("SEL  BRANCH · CRIADOR"))
		for _, b := range m.visible() {
			mark := "[ ]"
			if m.selected[b.Name] {
				mark = "[x]"
			}
			creator := b.Creator.UniqueName
			if creator == "" {
				creator = b.Creator.DisplayName
			}
			if creator == "" {
				creator = "criador desconhecido"
			}
			rows = append(rows, mark+" "+strings.TrimPrefix(b.Name, "refs/heads/")+" · "+creator)
		}
		help = "espaço seleccionar · / nome · u criador · c limpar · enter rever · r actualizar · b repos · q sair"
	case "review":
		lines = append(lines, "REVER ELIMINAÇÃO · branches remotas", "Confirma o repositório, os nomes e os SHAs.", "Isto não confirma que os commits já foram integrados.", "")
		for _, b := range m.reviewed {
			status := "sem bloqueios detectados"
			if b.Blocked != "" {
				status = "BLOQUEADA: " + b.Blocked
			}
			rows = append(rows, b.Name+" · "+b.ObjectID+" · "+status)
		}
		help = "↑/↓ rever lista · esc voltar · enter confirmar"
	case "results":
		lines = append(lines, "RESULTADOS · sem repetição automática", "")
		rows = m.results
		help = "↑/↓ navegar · r actualizar branches · b repos · q sair"
	}
	capacity := max(1, m.height-len(lines)-14)
	start := max(0, m.cursor-capacity+1)
	end := min(len(rows), start+capacity)
	for i := start; i < end; i++ {
		line := ansi.Truncate(rows[i], width-2, "…")
		if i == m.cursor {
			line = catalogActiveStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	if len(rows) == 0 && !m.busy {
		lines = append(lines, "Sem resultados.")
	}
	lines = append(lines, "", fmt.Sprintf("%d/%d · %d seleccionadas (inclui ocultas pelo filtro)", min(m.cursor+1, len(rows)), len(rows), len(m.selected)))
	if m.cursor < len(rows) {
		detail := rows[m.cursor]
		if m.stage == "review" {
			b := m.reviewed[m.cursor]
			detail = "Branch: " + b.Name + "\nSHA: " + b.ObjectID + "\nCriador: " + b.Creator.DisplayName + " " + b.Creator.UniqueName + "\nBloqueio: " + b.Blocked
		}
		wrapped := strings.Split(ansi.Wrap(detail, width, ""), "\n")
		offset := min(m.detailOffset, max(0, len(wrapped)-4))
		lines = append(lines, "Detalhe · ←/→ deslocar")
		lines = append(lines, wrapped[offset:min(len(wrapped), offset+4)]...)
	}
	if m.stage == "review" {
		lines = append(lines, m.confirmation.View())
	}
	if m.busy {
		lines = append(lines, "A processar… Esc cancela o restante lote.")
	}
	if m.err != "" {
		lines = append(lines, catalogWarningStyle.Render(ansi.Truncate(m.err, width, "…")))
	}
	if m.demo {
		lines = append(lines, "DEMO OFFLINE · não elimina branches")
	}
	lines = append(lines, strings.Split(ansi.Wrap(help, width, ""), "\n")...)
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "…")
	}
	return strings.Join(lines, "\n")
}
