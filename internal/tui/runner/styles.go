package runner

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	catalogTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	catalogHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117")).Background(lipgloss.Color("17"))
	catalogActiveStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("24"))
	catalogDetailStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "250"})
	catalogWarningStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	catalogFooterStyle  = catalogDetailStyle
	planStyle           = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	successStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	runStyle            = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	brandStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("234")).Background(lipgloss.Color("81")).Padding(0, 1)
	keyStyle            = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	borderStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	stripeStyle         = lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{Light: "254", Dark: "235"})
	wordmarkStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("232")).Background(lipgloss.Color("190")).Padding(0, 2)
	brandLimeStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("190"))
	brandWhiteStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "232", Dark: "231"})
)

// A connected-node signature doubles as a location indicator, not run progress.
func pipelineBrand(width, active int, offline bool) string {
	if width <= 0 {
		width = defaultWidth
	}
	brand := wordmarkStyle.Render("◆━ AZPIPE")
	names := []string{"Seleccionar", "Rever", "Acompanhar"}
	if width < 76 {
		names = []string{"Selecção", "Revisão", "Lote"}
	}
	steps := make([]string, 3)
	for i, name := range names {
		if i == active {
			steps[i] = keyStyle.Render("◉ " + name)
		} else {
			steps[i] = catalogDetailStyle.Render("○ " + name)
		}
	}
	line := brand + "  " + strings.Join(steps, borderStyle.Render(" ━━ "))
	if offline && lipgloss.Width(line)+10 <= width {
		line += "  " + planStyle.Render("OFFLINE")
	}
	return ansi.Truncate(line, width, "…")
}

func welcomeBrand() string {
	// Fixed five-row lettering needs no font dependency and fits an 80-column terminal.
	az := []string{" ███   █████", "█   █     █ ", "█████    █  ", "█   █   █   ", "█   █  █████"}
	pipe := []string{"████   █████  ████   █████", "█   █    █    █   █  █    ", "████     █    ████   ████ ", "█        █    █      █    ", "█      █████  █      █████"}
	lines := []string{wordmarkStyle.Render("› AZPIPE") + "  " + catalogDetailStyle.Render("AZURE DEVOPS / TUI"), ""}
	for i := range az {
		lines = append(lines, brandWhiteStyle.Render(az[i])+"  "+brandLimeStyle.Render(pipe[i]))
	}
	return strings.Join(append(lines,
		brandLimeStyle.Render(strings.Repeat("▪", 41)),
		brandWhiteStyle.Render("As tuas pipelines. ")+brandLimeStyle.Render("Um só terminal."),
		catalogDetailStyle.Render("Selecciona, revê e acompanha execuções em paralelo.")), "\n")
}

func section(title, body string, width int) string {
	if width <= 0 {
		width = defaultWidth
	}
	width = max(8, width)
	label := truncateWidth(" "+title+" ", width-4)
	lines := []string{borderStyle.Render("╭─" + label + strings.Repeat("─", max(0, width-3-lipgloss.Width(label))) + "╮")}
	for _, line := range strings.Split(body, "\n") {
		line = ansi.Truncate(line, width-4, "…")
		lines = append(lines, borderStyle.Render("│ ")+line+strings.Repeat(" ", max(0, width-4-lipgloss.Width(line)))+borderStyle.Render(" │"))
	}
	lines = append(lines, borderStyle.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return strings.Join(lines, "\n")
}

func tableCells(widths []int, values ...string) string {
	cells := make([]string, len(widths))
	for i, width := range widths {
		text := ""
		if i < len(values) {
			text = truncateWidth(values[i], width)
		}
		cells[i] = text + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
	}
	return strings.Join(cells, " │ ")
}

// Keep labels as well as colour, including in terminals with NO_COLOR.
func modeStyle(mode string) lipgloss.Style {
	if mode == "PLAN" {
		return planStyle
	}
	return runStyle
}

func shortcutBar(width int, items ...string) string {
	if width <= 0 {
		width = defaultWidth
	}
	var rows []string
	line := ""
	for _, item := range items {
		parts := strings.SplitN(item, " ", 2)
		label := keyStyle.Render(parts[0])
		if len(parts) == 2 {
			label += " " + catalogDetailStyle.Render(parts[1])
		}
		if line != "" && lipgloss.Width(line)+3+lipgloss.Width(label) > width {
			rows = append(rows, line)
			line = ""
		}
		if line != "" {
			line += "   "
		}
		line += label
	}
	return strings.Join(append(rows, line), "\n")
}
