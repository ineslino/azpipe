package runner

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	width = max(1, width-4)
	diagnostic := ""
	if m.err != "" {
		diagnostic = " · " + catalogWarningStyle.Render(m.err)
	}
	inputWidth := max(1, width-lipgloss.Width("Comando "+diagnostic))
	m.input.Width = max(1, inputWidth-lipgloss.Width(m.input.Prompt)-1)
	// Recalculate the viewport at this width while retaining the editing cursor.
	position := m.input.Position()
	m.input.CursorEnd()
	m.input.SetCursor(position)
	return truncateWidth("Comando "+truncateWidth(m.input.View(), inputWidth)+diagnostic, width)
}
