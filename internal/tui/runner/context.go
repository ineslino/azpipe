package runner

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ineslino/azpipe/internal/azdo"
)

// ClientFactory creates an authenticated client after the operator submits an organization.
type ClientFactory func(organization string) (azdo.Client, error)

// ContextDefaults pre-fills non-secret context values from flags or configuration.
type ContextDefaults struct {
	Organization string
	Project      string
}

type contextSubmitMsg struct {
	organization string
	project      string
}

type contextLoadedMsg struct {
	token        operationToken
	organization string
	client       azdo.Client
	project      string
	pipelines    []azdo.Pipeline
	err          error
}

type contextModel struct {
	organization textinput.Model
	project      textinput.Model
	focus        int
	err          string
}

func newContextModel(defaults ContextDefaults) contextModel {
	organization := textinput.New()
	organization.Prompt = "Organização: "
	organization.CharLimit = 256
	organization.Width = 48
	organization.PromptStyle = keyStyle
	organization.SetValue(defaults.Organization)
	organization.Focus()

	project := textinput.New()
	project.Prompt = "Projecto: "
	project.CharLimit = 256
	project.Width = 48
	project.PromptStyle = keyStyle
	project.SetValue(defaults.Project)

	return contextModel{organization: organization, project: project}
}

func (m contextModel) update(msg tea.Msg) (contextModel, tea.Cmd) {
	key, isKey := msg.(tea.KeyMsg)
	if isKey {
		switch key.String() {
		case "ctrl+c", "ctrl+d", "esc":
			return m, tea.Quit
		case "tab", "down":
			m.setFocus(1)
			return m, nil
		case "shift+tab", "up":
			m.setFocus(0)
			return m, nil
		case "enter":
			organization := strings.TrimSpace(m.organization.Value())
			project := strings.TrimSpace(m.project.Value())
			if organization == "" || project == "" {
				m.err = "Organização e projecto são obrigatórios."
				if organization != "" {
					m.setFocus(1)
				}
				return m, nil
			}
			m.err = ""
			return m, func() tea.Msg {
				return contextSubmitMsg{organization: organization, project: project}
			}
		}
	}

	var cmd tea.Cmd
	if m.focus == 0 {
		m.organization, cmd = m.organization.Update(msg)
	} else {
		m.project, cmd = m.project.Update(msg)
	}
	return m, cmd
}

func (m *contextModel) setFocus(focus int) {
	m.focus = focus
	if focus == 0 {
		m.project.Blur()
		m.organization.Focus()
		return
	}
	m.organization.Blur()
	m.project.Focus()
}

func (m contextModel) view() string {
	lines := []string{
		welcomeBrand(),
		"",
		catalogTitleStyle.Render("Contexto Azure DevOps"),
		"Introduza a organização e o projecto. A autenticação é lida da configuração local.",
		m.organization.View(),
		m.project.View(),
	}
	if m.err != "" {
		lines = append(lines, catalogWarningStyle.Render(m.err))
	}
	lines = append(lines, "", shortcutBar(defaultWidth, "tab mudar campo", "enter continuar", "esc sair"))
	return strings.Join(lines, "\n")
}

func loadContext(factory ClientFactory, organization, project string, token operationToken) tea.Cmd {
	return func() tea.Msg {
		if factory == nil {
			return contextLoadedMsg{token: token, organization: organization, project: project, err: fmt.Errorf("não foi possível criar o cliente: factory indisponível")}
		}
		client, err := factory(organization)
		if err != nil {
			return contextLoadedMsg{token: token, organization: organization, project: project, err: fmt.Errorf("não foi possível criar o cliente: %w", err)}
		}
		pipelines, err := client.ListPipelines(context.Background(), project)
		if err != nil {
			return contextLoadedMsg{token: token, organization: organization, project: project, err: fmt.Errorf("não foi possível listar pipelines: %w", err)}
		}
		return contextLoadedMsg{token: token, organization: organization, client: client, project: project, pipelines: pipelines}
	}
}
