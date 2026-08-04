package runner

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
}

func newReviewModel(selections []domainrunner.Selection, demo bool, token operationToken) reviewModel {
	reviews := make([]domainrunner.Review, len(selections))
	for index, selection := range selections {
		reviews[index] = domainrunner.Review{Selection: selection, State: domainrunner.ReviewPending}
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
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
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

func (m reviewModel) view() string {
	lines := []string{
		catalogTitleStyle.Render("Revisão"),
		catalogHeaderStyle.Render("ESTADO MODE    ID PIPELINE BRANCH PARÂMETROS"),
	}
	for _, review := range m.reviews {
		state := string(review.State)
		if m.demo {
			state = "DEMO"
		}
		request := review.Selection.Request()
		line := fmt.Sprintf("%-6s %-4s %5d %s %s %s", state, review.Selection.Mode, review.Selection.Pipeline.ID, review.Selection.Pipeline.Name, request.Branch, formatParameters(request.Parameters))
		if review.Err != nil {
			line += ": " + review.Err.Error()
		}
		lines = append(lines, line)
	}
	if m.demo {
		lines = append(lines, catalogDetailStyle.Render("Demo offline: nenhuma pipeline será executada."))
	} else if m.canExecute() {
		lines = append(lines, "Escreva EXECUTAR para colocar todas as pipelines em execução.", m.confirmation.View())
	}
	if m.warning != "" {
		lines = append(lines, catalogWarningStyle.Render(m.warning))
	}
	lines = append(lines, catalogFooterStyle.Render("esc voltar • q sair"))
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
