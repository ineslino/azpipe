package runner

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ineslino/azpipe/internal/azdo"
	domainrunner "github.com/ineslino/azpipe/internal/runner"
)

const confirmationValue = "EXECUTAR"

type previewFinishedMsg struct {
	token   operationToken
	reviews []domainrunner.Review
}

type previewProgressMsg struct {
	token  operationToken
	index  int
	review domainrunner.Review
	next   <-chan tea.Msg
}

type queueConfirmedMsg struct {
	token   operationToken
	reviews []domainrunner.Review
}

type reviewModel struct {
	reviews      []domainrunner.Review
	confirmation textinput.Model
	demo         bool
	warning      string
	token        operationToken
	offset       int
	height       int
	width        int
	horizontal   int
	previewing   bool
	cancelled    bool
	organization string
	project      string
}

func newReviewModel(selections []domainrunner.Selection, demo bool, token operationToken) reviewModel {
	reviews := make([]domainrunner.Review, len(selections))
	for index, selection := range selections {
		reviews[index] = domainrunner.Review{Selection: selection, State: domainrunner.ReviewPending}
		if demo {
			request := selection.Request()
			request.Commit = "0123456789012345678901234567890123456789"
			request.DefinitionVersion = 7
			reviews[index].Request = request
		}
	}
	confirmation := textinput.New()
	confirmation.Prompt = "Confirmação: "
	confirmation.CharLimit = len(confirmationValue)
	confirmation.Width = 24
	return reviewModel{reviews: reviews, confirmation: confirmation, demo: demo, token: token}
}

func previewSelections(ctx context.Context, service domainrunner.Service, selections []domainrunner.Selection, token operationToken) tea.Cmd {
	return func() tea.Msg {
		events := make(chan tea.Msg, len(selections)+1)
		service.OnPreview = func(index int, review domainrunner.Review) {
			events <- previewProgressMsg{token: token, index: index, review: review, next: events}
		}
		go func() {
			reviews := service.PreviewAll(ctx, selections, 4)
			events <- previewFinishedMsg{token: token, reviews: reviews}
			close(events)
		}()
		return <-events
	}
}

func (m reviewModel) update(msg tea.Msg) (reviewModel, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.height = size.Height
		m.width = size.Width
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "right":
			m.horizontal++
			return m, nil
		case "left":
			m.horizontal = max(0, m.horizontal-1)
			return m, nil
		case "pgdown":
			m.offset = min(max(0, len(m.reviews)-1), m.offset+m.listCapacity())
			m.horizontal = 0
			return m, nil
		case "pgup":
			m.offset = max(0, m.offset-m.listCapacity())
			m.horizontal = 0
			return m, nil
		case "down", "up":
			if key.String() == "down" {
				m.offset = min(max(0, len(m.reviews)-1), m.offset+1)
			} else {
				m.offset = max(0, m.offset-1)
			}
			m.horizontal = 0
			return m, nil
		case "ctrl+c", "ctrl+d":
			return m, tea.Quit
		case "enter":
			if m.canExecute() && m.confirmation.Value() == confirmationValue {
				return m, func() tea.Msg { return queueConfirmedMsg{token: m.token, reviews: m.reviews} }
			}
			if m.canExecute() {
				m.warning = "Escreva EXECUTAR exactamente para confirmar."
			}
			return m, nil
		}
	}
	if !m.canExecute() {
		return m, nil
	}
	var cmd tea.Cmd
	m.confirmation, cmd = m.confirmation.Update(msg)
	return m, cmd
}

func (m reviewModel) canExecute() bool {
	if m.demo || m.previewing || m.cancelled || len(m.reviews) == 0 {
		return false
	}
	for _, review := range m.reviews {
		if review.State != domainrunner.ReviewReady || review.Err != nil {
			return false
		}
	}
	return true
}

func (m reviewModel) listCapacity() int {
	height := m.height
	if height == 0 {
		height = defaultHeight
	}
	if height >= 28 {
		return max(1, height-21)
	}
	return max(1, height-17)
}

func (m reviewModel) view() string {
	width, height := m.width, m.height
	if width == 0 {
		width = defaultWidth
	}
	if height == 0 {
		height = defaultHeight
	}
	ready, blocked := 0, 0
	for _, r := range m.reviews {
		if r.Err != nil {
			blocked++
		} else if r.State == domainrunner.ReviewReady {
			ready++
		}
	}
	pipelines := make([]azdo.Pipeline, len(m.reviews))
	for i, review := range m.reviews {
		pipelines[i] = review.Selection.Pipeline
	}
	includeProject := hasMultiplePipelineProjects(pipelines)
	columns := []int{2, 9, 4, 5, max(1, width-32)}
	headers := []string{"", "ESTADO", "MODO", "ID", "PIPELINE"}
	if includeProject && width >= 70 {
		columns = []int{2, 9, 4, 5, 15, max(1, width-50)}
		headers = []string{"", "ESTADO", "MODO", "ID", "PROJECTO", "PIPELINE"}
	} else if includeProject {
		columns = []int{2, 9, 4, 5, 12, max(1, width-47)}
		headers = []string{"", "ESTADO", "MODO", "ID", "PROJECTO", "PIPELINE"}
	}
	lines := []string{catalogTitleStyle.Render(fmt.Sprintf("Revisão · %s · %s · %s", quantity(ready, "pronta", "prontas"), quantity(blocked, "bloqueada", "bloqueadas"), quantity(len(m.reviews)-ready-blocked, "pendente", "pendentes"))), catalogHeaderStyle.Width(width).Render(tableCells(columns, headers...))}
	if m.demo {
		lines[0] = catalogTitleStyle.Render(fmt.Sprintf("Revisão de exemplo · %s · sem execução remota", quantity(len(m.reviews), "pipeline", "pipelines")))
	}
	if height >= 28 {
		lines = []string{"", lines[0], "", lines[1]}
	}
	start := m.offset / m.listCapacity() * m.listCapacity()
	end := min(len(m.reviews), start+m.listCapacity())
	for i := start; i < end; i++ {
		r := m.reviews[i]
		state := string(r.State)
		switch r.State {
		case domainrunner.ReviewPending:
			state = "Pendente"
		case domainrunner.ReviewReady:
			state = "Pronta"
		case domainrunner.ReviewError:
			state = "Bloqueada"
		}
		if m.demo {
			state = "DEMO"
		}
		marker := " "
		if i == m.offset {
			marker = ">"
		}
		values := []string{marker, state, string(r.Selection.Mode), fmt.Sprint(r.Selection.ID()), r.Selection.Pipeline.Name}
		if includeProject {
			values = []string{marker, state, string(r.Selection.Mode), fmt.Sprint(r.Selection.ID()), r.Selection.Pipeline.Project, r.Selection.Pipeline.Name}
		}
		accents := map[int]lipgloss.Style{2: modeStyle(string(r.Selection.Mode))}
		if r.Err != nil {
			accents[1] = catalogWarningStyle
		} else if r.State == domainrunner.ReviewReady {
			accents[1] = successStyle
		}
		lines = append(lines, renderTableRow(columns, values, i, i == m.offset, accents))
	}
	lines = append(lines, catalogDetailStyle.Render(fmt.Sprintf("  %d–%d de %d · ↑/↓ escolher pipeline", min(start+1, len(m.reviews)), end, len(m.reviews))))
	if height >= 28 {
		lines = append(lines, "")
	}
	if len(m.reviews) > 0 {
		r := m.reviews[m.offset]
		request := r.Request
		if request.PipelineID == 0 {
			request = r.Selection.Request()
		}
		detail := fmt.Sprintf("Modo: %s\nBranch: %s\nParâmetros enviados: %s\n\nSHA: %s\nDefinição: %d\nValores predefinidos: definidos pela pipeline", r.Selection.Mode, request.Branch, formatParameters(request.Parameters), request.Commit, request.DefinitionVersion)
		if !m.demo {
			project := r.Selection.Project()
			if project == "" {
				project = m.project
			}
			detail += "\nOrganização: " + m.organization + "\nProjecto: " + project
		}
		if r.Err != nil {
			detail = "Bloqueio: " + r.Err.Error() + "\n" + detail
		}
		wrapped := strings.Split(ansi.Wrap(detail, width, ""), "\n")
		scroll := min(m.horizontal, max(0, len(wrapped)-5))
		lines = append(lines, catalogHeaderStyle.Width(width).Render(truncateWidth("── Detalhe · "+pipelineDisplayName(r.Selection.Pipeline, includeProject), width)))
		if height >= 28 {
			lines = append(lines, "")
		}
		for _, line := range wrapped[scroll:min(len(wrapped), scroll+5)] {
			lines = append(lines, catalogDetailStyle.Render(line))
		}
		lines = append(lines, catalogDetailStyle.Render(fmt.Sprintf("Detalhe %d–%d/%d · ←/→ deslocar", scroll+1, min(len(wrapped), scroll+5), len(wrapped))))
	}
	lines = append(lines, "")
	if m.demo {
		lines = append(lines, catalogDetailStyle.Render("Demo offline: nenhuma pipeline será executada."), shortcutBar(width, "enter ver exemplo de acompanhamento (simulação)"))
	} else if m.canExecute() {
		lines = append(lines, runStyle.Render(fmt.Sprintf("Vai lançar %s. Escreve EXECUTAR para confirmar.", quantity(len(m.reviews), "pipeline", "pipelines"))), m.confirmation.View())
	} else {
		if m.cancelled {
			lines = append(lines, catalogWarningStyle.Render("Preview cancelada. Esc volta à lista; :q sai."))
		} else if blocked > 0 && !m.previewing {
			lines = append(lines, catalogWarningStyle.Render("Escolhe uma pipeline com erro. Enter volta à lista para corrigir."))
		} else {
			lines = append(lines, catalogDetailStyle.Render(fmt.Sprintf("Previews: %d/%d concluídas · Esc ou :q cancela.", ready+blocked, len(m.reviews))))
		}
	}
	if m.warning != "" {
		lines = append(lines, catalogWarningStyle.Render(m.warning))
	}
	quit := ":q sair"
	if m.previewing {
		quit = ":q cancelar preview"
	}
	if m.confirmation.Focused() {
		quit = "←/→ detalhe completo"
	}
	lines = append(lines, shortcutBar(width, "pgup/pgdown página", "esc voltar e editar", quit))
	return strings.Join(lines, "\n")
}

func formatParameters(parameters map[string]string) string {
	if len(parameters) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(parameters))
	for key := range parameters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+"="+parameters[key])
	}
	return strings.Join(values, ",")
}
