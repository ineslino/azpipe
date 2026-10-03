package runner

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type commandAction int

const (
	commandHandled commandAction = iota
	commandQuit
)

type commandModel struct {
	input  textinput.Model
	active bool
	err    string
}

func newCommandModel() commandModel {
	input := textinput.New()
	input.Prompt = ":"
	input.CharLimit = 64
	input.Width = 32
	input.PromptStyle = keyStyle
	return commandModel{input: input}
}

func (m *commandModel) start(width int) tea.Cmd {
	if m.input.Prompt == "" {
		*m = newCommandModel()
	}
	m.active = true
	m.err = ""
	m.input.SetValue("")
	m.input.Width = max(12, width-10)
	return m.input.Focus()
}

func (m *commandModel) startFromKey(key tea.KeyMsg, width int) (tea.Cmd, bool) {
	if key.Type != tea.KeyRunes || key.Alt || len(key.Runes) == 0 || key.Runes[0] != ':' {
		return nil, false
	}
	cmd := m.start(width)
	m.input.SetValue(string(key.Runes[1:]))
	return cmd, true
}

func (m *commandModel) update(key tea.KeyMsg) (commandAction, tea.Cmd) {
	if !m.active {
		return commandHandled, nil
	}
	if key.Type == tea.KeyCtrlC || key.Type == tea.KeyCtrlD {
		return commandQuit, nil
	}
	switch key.Type {
	case tea.KeyEsc:
		m.close()
		return commandHandled, nil
	case tea.KeyEnter:
		command := strings.TrimSpace(m.input.Value())
		if command == "q" || command == "quit" {
			return commandQuit, nil
		}
		m.err = "Comando desconhecido. Usa :q para sair."
		return commandHandled, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(key)
	return commandHandled, cmd
}

func (m *commandModel) close() {
	m.active = false
	m.err = ""
	m.input.Blur()
}

func (m commandModel) view(width int) string {
	if !m.active {
		return ""
	}
	value := m.input.View()
	if m.err != "" {
		value += " · " + catalogWarningStyle.Render(m.err)
	}
	return truncateWidth("Comando "+value, max(1, width-4))
}
