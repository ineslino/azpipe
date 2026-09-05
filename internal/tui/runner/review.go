package runner

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	domainrunner "github.com/ineslino/azpipe/internal/runner"
)

const confirmationValue = "EXECUTAR"

type previewFinishedMsg struct {
	token   operationToken
	reviews []domainrunner.Review
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

func previewSelections(service domainrunner.Service, selections []domainrunner.Selection, token operationToken) tea.Cmd {
	return func() tea.Msg {
		return previewFinishedMsg{token: token, reviews: service.PreviewAll(context.Background(), selections, 4)}
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
		case "ctrl+c", "ctrl+d", "q":
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
	if m.demo || len(m.reviews) == 0 {
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
	columns := []int{2, 8, 4, 5, max(1, width-31)}
	lines := []string{catalogTitleStyle.Render(fmt.Sprintf("Revisão · %d pipelines · %d prontas · %d bloqueadas", len(m.reviews), ready, blocked)), catalogHeaderStyle.Width(width).Render(tableCells(columns, "", "ESTADO", "MODO", "ID", "PIPELINE"))}
	start := m.offset / m.listCapacity() * m.listCapacity()
	end := min(len(m.reviews), start+m.listCapacity())
	for i := start; i < end; i++ {
		r := m.reviews[i]
		state := string(r.State)
		if m.demo {
			state = "DEMO"
		}
		marker := " "
		if i == m.offset {
			marker = ">"
		}
		line := tableCells(columns, marker, state, string(r.Selection.Mode), fmt.Sprint(r.Selection.ID()), r.Selection.Pipeline.Name)
		style := modeStyle(string(r.Selection.Mode))
		if r.Err != nil {
			style = catalogWarningStyle
		}
		if i == m.offset {
			style = catalogActiveStyle.Width(width)
		}
		lines = append(lines, style.Render(line))
	}
	lines = append(lines, catalogDetailStyle.Render(fmt.Sprintf("  %d–%d de %d · ↑/↓ escolher pipeline", min(start+1, len(m.reviews)), end, len(m.reviews))))
	if len(m.reviews) > 0 {
		r := m.reviews[m.offset]
		request := r.Request
		if request.PipelineID == 0 {
			request = r.Selection.Request()
		}
		detail := fmt.Sprintf("Modo: %s · Branch: %s\nPARÂMETROS ENVIADOS: %s\nDefaults não enviados: definidos pela pipeline\nSHA: %s\nDefinição: %d", r.Selection.Mode, request.Branch, formatParameters(request.Parameters), request.Commit, request.DefinitionVersion)
		if r.Err != nil {
			detail = "Bloqueio: " + r.Err.Error() + "\n" + detail
		}
		wrapped := strings.Split(ansi.Wrap(detail, width, ""), "\n")
		scroll := min(m.horizontal, max(0, len(wrapped)-5))
		lines = append(lines, catalogHeaderStyle.Render(truncateWidth("── Detalhe · "+r.Selection.Pipeline.Name, width)))
		for _, line := range wrapped[scroll:min(len(wrapped), scroll+5)] {
			lines = append(lines, catalogDetailStyle.Render(line))
		}
		lines = append(lines, catalogDetailStyle.Render(fmt.Sprintf("Detalhe %d–%d/%d · ←/→ deslocar", scroll+1, min(len(wrapped), scroll+5), len(wrapped))))
	}
	for len(lines) < height-7 {
		lines = append(lines, "")
	}
	if m.demo {
		lines = append(lines, catalogDetailStyle.Render("Demo offline: nenhuma pipeline será executada."))
	} else if m.canExecute() {
		lines = append(lines, runStyle.Render(fmt.Sprintf("Vai lançar %d pipelines. Escreva EXECUTAR para confirmar.", len(m.reviews))), m.confirmation.View())
	} else {
		if blocked > 0 {
			lines = append(lines, catalogWarningStyle.Render("Escolhe uma pipeline com erro. Enter volta à lista para corrigir."))
		} else {
			lines = append(lines, catalogDetailStyle.Render("A validar a selecção. Aguarda as previews; Esc permite voltar."))
		}
	}
	if m.warning != "" {
		lines = append(lines, catalogWarningStyle.Render(m.warning))
	}
	lines = append(lines, shortcutBar(width, "pgup/pgdown página", "esc voltar e editar", "q sair"))
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
