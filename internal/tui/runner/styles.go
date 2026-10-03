package runner

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func quantity(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

var (
	// Keep the lime/cyan identity; use adaptive neutrals for terminal backgrounds.
	accentColor         = lipgloss.AdaptiveColor{Light: "22", Dark: "190"}
	focusColor          = lipgloss.AdaptiveColor{Light: "25", Dark: "81"}
	textColor           = lipgloss.AdaptiveColor{Light: "235", Dark: "254"}
	mutedColor          = lipgloss.AdaptiveColor{Light: "241", Dark: "246"}
	surfaceColor        = lipgloss.AdaptiveColor{Light: "254", Dark: "235"}
	catalogTextStyle    = lipgloss.NewStyle().Foreground(textColor)
	catalogTitleStyle   = catalogTextStyle.Bold(true)
	catalogHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(focusColor).Background(surfaceColor)
	catalogActiveStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("24"))
	catalogDetailStyle  = lipgloss.NewStyle().Foreground(mutedColor)
	catalogWarningStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "124", Dark: "210"})
	catalogFooterStyle  = catalogDetailStyle
	planStyle           = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "91", Dark: "183"})
	successStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "22", Dark: "84"})
	runStyle            = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "94", Dark: "221"})
	brandStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("234")).Background(lipgloss.Color("81")).Padding(0, 1)
	keyStyle            = lipgloss.NewStyle().Bold(true).Foreground(focusColor)
	shortcutKeyStyle    = keyStyle.Background(surfaceColor)
	borderStyle         = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "245", Dark: "240"})
	stripeStyle         = catalogTextStyle.Background(lipgloss.AdaptiveColor{Light: "255", Dark: "234"})
	wordmarkStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("232")).Background(lipgloss.Color("190")).Padding(0, 2)
	brandLimeStyle      = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
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

// A stable full-screen height prevents overlay closure from erasing unchanged borders.
func terminalView(view string, height int) string {
	return view + strings.Repeat("\n", max(0, height-lipgloss.Height(view)))
}

func section(title, body string, width int) string {
	if width <= 0 {
		width = defaultWidth
	}
	width = max(8, width)
	label := truncateWidth(" "+title+" ", width-4)
	lines := []string{borderStyle.Render("╭─") + keyStyle.Render(label) + borderStyle.Render(strings.Repeat("─", max(0, width-3-lipgloss.Width(label)))+"╮")}
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
			text = ansi.Truncate(values[i], width, "…")
		}
		cells[i] = text + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
	}
	return strings.Join(cells, " │ ")
}

// Focus spans the row; semantic colours belong to individual cells.
func renderTableRow(widths []int, values []string, index int, active bool, accents map[int]lipgloss.Style) string {
	style := catalogTextStyle
	if active {
		style = catalogActiveStyle
	} else if index%2 == 0 {
		style = stripeStyle
	}
	cells := make([]string, len(widths))
	for i, width := range widths {
		cellStyle := style
		if accent, ok := accents[i]; ok && !active {
			cellStyle = cellStyle.Foreground(accent.GetForeground()).Bold(accent.GetBold())
		}
		value := ""
		if i < len(values) {
			value = truncateWidth(values[i], width)
		}
		cells[i] = cellStyle.Width(width).Render(value)
	}
	return strings.Join(cells, style.Render(" │ "))
}

func metadata(label, value string) string {
	if value == "" {
		value = "n/d"
	}
	return catalogDetailStyle.Render(label+": ") + catalogTextStyle.Render(value)
}

// textPage keeps wrapped diagnostics readable without losing the recovery footer.
func textPage(value string, width, offset, count int) string {
	lines := strings.Split(ansi.Wrap(value, max(1, width), ""), "\n")
	start := min(max(0, offset), max(0, len(lines)-count))
	return strings.Join(lines[start:min(len(lines), start+count)], "\n")
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
		label := shortcutKeyStyle.Render(parts[0])
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
